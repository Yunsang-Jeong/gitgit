package app

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

type SearchOptions struct {
	Predicates   []SearchPredicate
	Messages     []string
	Diffs        []string
	Files        []string
	Engine       string
	FollowRename bool
	Revision     string
	All          bool
	Paths        []string
	Author       string
	Since        string
	Until        string
	Limit        int
	Context      int
}

type SearchPredicate struct {
	Source string
	Value  string
	Join   string
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type FileChange struct {
	Status  string `json:"status"`
	OldPath string `json:"old_path,omitempty"`
	Path    string `json:"path"`
}

type SearchMatch struct {
	Author       Author     `json:"author"`
	Commit       string     `json:"commit"`
	File         FileChange `json:"file"`
	Diff         string     `json:"diff"`
	MatchSources []string   `json:"match_sources,omitempty"`
}

type SearchResponse struct {
	MessagePatterns []string      `json:"message_patterns,omitempty"`
	DiffPatterns    []string      `json:"diff_patterns,omitempty"`
	FilePatterns    []string      `json:"file_patterns,omitempty"`
	Engine          string        `json:"engine"`
	FollowRename    bool          `json:"follow_rename,omitempty"`
	Results         []SearchMatch `json:"results"`
	Count           int           `json:"count"`
}

type SearchService struct {
	repo *gitexec.Repository
}

func NewSearchService(repo *gitexec.Repository) *SearchService {
	return &SearchService{repo: repo}
}

type commitMeta struct {
	OID     string
	Author  Author
	Message string
}

type compiledSearchPredicate struct {
	SearchPredicate
	matcher      func(string) bool
	trackedPaths map[string]bool
}

func (s *SearchService) Search(ctx context.Context, options SearchOptions) (SearchResponse, error) {
	if options.Engine == "" {
		options.Engine = "glob"
	}
	if err := validateSearchOptions(options); err != nil {
		return SearchResponse{}, err
	}
	predicates, err := compileSearchPredicates(options)
	if err != nil {
		return SearchResponse{}, err
	}
	hasDiffPredicate := false
	for _, predicate := range predicates {
		if predicate.Source == "diff" {
			hasDiffPredicate = true
			break
		}
	}
	commits, err := s.listCommits(ctx, options)
	if err != nil {
		return SearchResponse{}, searchGitError("list history", err)
	}
	var eligibleCommits map[string]bool
	if options.FollowRename && (options.Author != "" || options.Since != "" || options.Until != "") {
		eligibleCommits = make(map[string]bool, len(commits))
		for _, commit := range commits {
			eligibleCommits[commit.OID] = true
		}
		traversalOptions := options
		traversalOptions.Author = ""
		traversalOptions.Since = ""
		traversalOptions.Until = ""
		commits, err = s.listCommits(ctx, traversalOptions)
		if err != nil {
			return SearchResponse{}, searchGitError("list rename traversal history", err)
		}
	}

	results := make([]SearchMatch, 0, min(options.Limit, 32))
	for _, commit := range commits {
		changes, changeErr := s.fileChanges(ctx, commit.OID, options.Paths)
		if changeErr != nil {
			return SearchResponse{}, searchGitError("read changed files", changeErr)
		}
		for _, change := range changes {
			hits := make([]bool, len(predicates))
			for index := range predicates {
				switch predicates[index].Source {
				case "msg":
					hits[index] = predicates[index].matcher(commit.Message)
				case "file":
					hits[index] = matchFileChange(change, predicates[index].matcher, options.FollowRename, predicates[index].trackedPaths)
				}
			}
			if eligibleCommits != nil && !eligibleCommits[commit.OID] {
				continue
			}
			if !hasDiffPredicate && !evaluateSearchExpression(predicates, hits) {
				continue
			}
			diff, diffErr := s.fileDiff(ctx, commit.OID, change, options.Context)
			if diffErr != nil {
				return SearchResponse{}, searchGitError("read file diff", diffErr)
			}
			if hasDiffPredicate {
				lines := changedLines(diff)
				for index := range predicates {
					if predicates[index].Source == "diff" {
						hits[index] = predicates[index].matcher(lines)
					}
				}
			}
			if !evaluateSearchExpression(predicates, hits) {
				continue
			}
			sources := make([]string, 0, 3)
			for index, predicate := range predicates {
				if hits[index] && !slices.Contains(sources, predicate.Source) {
					sources = append(sources, predicate.Source)
				}
			}
			results = append(results, SearchMatch{
				Author: commit.Author, Commit: commit.OID, File: change,
				Diff: diff, MatchSources: sources,
			})
			if len(results) >= options.Limit {
				return searchResponse(options, results), nil
			}
		}
	}
	return searchResponse(options, results), nil
}

func validateSearchOptions(options SearchOptions) error {
	if options.All && options.Revision != "" {
		return apperr.New("invalid_arguments", "all refs and a revision scope cannot be used together", apperr.ExitUsage, nil)
	}
	if revision := strings.TrimSpace(options.Revision); strings.HasPrefix(revision, "-") {
		return apperr.New("invalid_revision", "revision scope cannot be a Git option", apperr.ExitUsage, map[string]any{"revision": revision})
	}
	hasFilePredicate := len(options.Files) > 0
	for _, predicate := range options.Predicates {
		if strings.EqualFold(strings.TrimSpace(predicate.Source), "file") {
			hasFilePredicate = true
		}
	}
	if options.FollowRename && !hasFilePredicate {
		return apperr.New("invalid_arguments", "rename following requires at least one file pattern", apperr.ExitUsage, nil)
	}
	if len(options.Predicates) == 0 && len(options.Messages) == 0 && len(options.Diffs) == 0 && len(options.Files) == 0 {
		return apperr.New("missing_search_pattern", "at least one message, diff, or file pattern is required", apperr.ExitUsage, nil)
	}
	switch options.Engine {
	case "glob", "regex":
	default:
		return apperr.New("invalid_engine", fmt.Sprintf("unsupported search engine %q", options.Engine), apperr.ExitUsage, map[string]any{"allowed": []string{"glob", "regex"}})
	}
	if options.FollowRename && len(options.Paths) > 0 {
		return apperr.New("invalid_arguments", "rename following cannot be combined with a path filter", apperr.ExitUsage, nil)
	}
	if options.Limit <= 0 {
		return apperr.New("invalid_limit", "limit must be greater than zero", apperr.ExitUsage, nil)
	}
	if options.Context < 0 {
		return apperr.New("invalid_context", "context must be zero or greater", apperr.ExitUsage, nil)
	}
	return nil
}

func (s *SearchService) listCommits(ctx context.Context, options SearchOptions) ([]commitMeta, error) {
	args := []string{"log", "--date-order", "-z", "--format=%H%x00%an%x00%ae%x00%B"}
	args = append(args, searchFilterArgs(options)...)
	args = append(args, searchScopeArgs(options)...)
	if len(options.Paths) > 0 {
		args = append(args, "--")
		args = append(args, options.Paths...)
	}
	out, err := s.repo.Run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(out, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%4 != 0 {
		return nil, fmt.Errorf("unexpected git log field count %d", len(fields))
	}
	commits := make([]commitMeta, 0, len(fields)/4)
	for index := 0; index < len(fields); index += 4 {
		commits = append(commits, commitMeta{
			OID: string(fields[index]),
			Author: Author{
				Name:  string(fields[index+1]),
				Email: string(fields[index+2]),
			},
			Message: strings.TrimRight(string(fields[index+3]), "\n"),
		})
	}
	return commits, nil
}

func searchScopeArgs(options SearchOptions) []string {
	if options.All {
		return []string{"--all"}
	}
	if options.Revision != "" {
		return []string{"--end-of-options", options.Revision}
	}
	return []string{"--end-of-options", "HEAD"}
}

func searchFilterArgs(options SearchOptions) []string {
	args := make([]string, 0, 3)
	if options.Author != "" {
		args = append(args, "--author="+options.Author)
	}
	if options.Since != "" {
		args = append(args, "--since-as-filter="+options.Since)
	}
	if options.Until != "" {
		args = append(args, "--until="+options.Until)
	}
	return args
}

func (s *SearchService) fileChanges(ctx context.Context, oid string, paths []string) ([]FileChange, error) {
	args := []string{"diff-tree", "--root", "--no-commit-id", "--name-status", "-r", "-z", "-M", oid}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := s.repo.Run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	tokens := bytes.Split(out, []byte{0})
	changes := make([]FileChange, 0, len(tokens)/2)
	for index := 0; index < len(tokens); {
		status := string(tokens[index])
		index++
		if status == "" {
			continue
		}
		if index >= len(tokens) {
			return nil, fmt.Errorf("missing path for status %q", status)
		}
		if status[0] == 'R' || status[0] == 'C' {
			if index+1 >= len(tokens) {
				return nil, fmt.Errorf("missing rename paths for status %q", status)
			}
			changes = append(changes, FileChange{Status: status, OldPath: string(tokens[index]), Path: string(tokens[index+1])})
			index += 2
			continue
		}
		changes = append(changes, FileChange{Status: status, Path: string(tokens[index])})
		index++
	}
	return changes, nil
}

func (s *SearchService) fileDiff(ctx context.Context, oid string, change FileChange, contextLines int) (string, error) {
	args := []string{
		"show", "--format=", "--no-ext-diff", "--no-color", "--find-renames",
		fmt.Sprintf("--unified=%d", contextLines), oid, "--",
	}
	if change.OldPath != "" {
		args = append(args, change.OldPath)
	}
	args = append(args, change.Path)
	out, err := s.repo.Run(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func searchResponse(options SearchOptions, results []SearchMatch) SearchResponse {
	messages, diffs, files := searchPatternsBySource(options)
	return SearchResponse{
		MessagePatterns: messages,
		DiffPatterns:    diffs,
		FilePatterns:    files,
		Engine:          options.Engine,
		FollowRename:    options.FollowRename,
		Results:         results,
		Count:           len(results),
	}
}

func compileSearchPredicates(options SearchOptions) ([]compiledSearchPredicate, error) {
	predicates, err := normalizedSearchPredicates(options)
	if err != nil {
		return nil, err
	}
	compiled := make([]compiledSearchPredicate, 0, len(predicates))
	for _, predicate := range predicates {
		matcher, matchErr := compileSearchMatcher(predicate.Value, options.Engine, predicate.Source)
		if matchErr != nil {
			return nil, matchErr
		}
		compiled = append(compiled, compiledSearchPredicate{
			SearchPredicate: predicate,
			matcher:         matcher,
			trackedPaths:    map[string]bool{},
		})
	}
	return compiled, nil
}

func normalizedSearchPredicates(options SearchOptions) ([]SearchPredicate, error) {
	predicates := options.Predicates
	if len(predicates) == 0 {
		predicates = make([]SearchPredicate, 0, len(options.Messages)+len(options.Diffs)+len(options.Files))
		for _, entry := range []struct {
			source string
			values []string
		}{
			{source: "msg", values: options.Messages},
			{source: "diff", values: options.Diffs},
			{source: "file", values: options.Files},
		} {
			for _, value := range entry.values {
				predicates = append(predicates, SearchPredicate{Source: entry.source, Value: value, Join: "or"})
			}
		}
	}

	normalized := make([]SearchPredicate, 0, len(predicates))
	for _, predicate := range predicates {
		source := strings.ToLower(strings.TrimSpace(predicate.Source))
		switch source {
		case "msg", "diff", "file":
		default:
			return nil, apperr.New("invalid_arguments", fmt.Sprintf("unsupported pattern source %q", predicate.Source), apperr.ExitUsage, nil)
		}
		join := strings.ToLower(strings.TrimSpace(predicate.Join))
		if len(normalized) == 0 {
			join = ""
		} else if join == "" {
			join = "or"
		} else if join != "and" && join != "or" {
			return nil, apperr.New("invalid_arguments", fmt.Sprintf("unsupported search join %q", predicate.Join), apperr.ExitUsage, nil)
		}
		normalized = append(normalized, SearchPredicate{Source: source, Value: strings.TrimSpace(predicate.Value), Join: join})
	}
	return normalized, nil
}

func evaluateSearchExpression(predicates []compiledSearchPredicate, hits []bool) bool {
	if len(predicates) == 0 || len(predicates) != len(hits) {
		return false
	}
	matched := false
	groupMatched := hits[0]
	for index := 1; index < len(predicates); index++ {
		if predicates[index].Join == "and" {
			groupMatched = groupMatched && hits[index]
			continue
		}
		matched = matched || groupMatched
		groupMatched = hits[index]
	}
	return matched || groupMatched
}

func searchPatternsBySource(options SearchOptions) (messages, diffs, files []string) {
	if len(options.Predicates) == 0 {
		return options.Messages, options.Diffs, options.Files
	}
	for _, predicate := range options.Predicates {
		switch strings.ToLower(strings.TrimSpace(predicate.Source)) {
		case "msg":
			messages = append(messages, strings.TrimSpace(predicate.Value))
		case "diff":
			diffs = append(diffs, strings.TrimSpace(predicate.Value))
		case "file":
			files = append(files, strings.TrimSpace(predicate.Value))
		}
	}
	return messages, diffs, files
}

func compileSearchMatchers(patterns []string, engine, source string) (func(string) bool, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	matchers := make([]func(string) bool, 0, len(patterns))
	for _, pattern := range patterns {
		matcher, err := compileSearchMatcher(pattern, engine, source)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, matcher)
	}
	return func(value string) bool {
		for _, matcher := range matchers {
			if matcher(value) {
				return true
			}
		}
		return false
	}, nil
}

func compileSearchMatcher(pattern, engine, source string) (func(string) bool, error) {
	if pattern == "" {
		return nil, apperr.New(
			"invalid_pattern", fmt.Sprintf("%s pattern cannot be empty", source), apperr.ExitUsage,
			map[string]any{"source": source, "pattern": pattern, "engine": engine},
		)
	}
	matcher, err := newMatcherMode(pattern, engine, source == "file")
	if err != nil {
		return nil, apperr.Wrap(
			"invalid_pattern", fmt.Sprintf("invalid %s pattern", source), apperr.ExitUsage, err,
			map[string]any{"source": source, "pattern": pattern, "engine": engine},
		)
	}
	return matcher, nil
}

func matchFileChange(change FileChange, matcher func(string) bool, followRename bool, trackedPaths map[string]bool) bool {
	if matcher == nil {
		return false
	}
	pathMatched := matcher(change.Path)
	if !followRename {
		return pathMatched
	}

	isRename := strings.HasPrefix(change.Status, "R") && change.OldPath != ""
	oldPathMatched := isRename && matcher(change.OldPath)
	pathTracked := trackedPaths[change.Path]
	oldPathTracked := isRename && trackedPaths[change.OldPath]
	matched := pathMatched || oldPathMatched || pathTracked || oldPathTracked
	if pathMatched {
		trackedPaths[change.Path] = true
	}
	if isRename && (pathMatched || pathTracked || oldPathMatched || oldPathTracked) {
		trackedPaths[change.OldPath] = true
	}
	return matched
}

func newMatcher(pattern, engine string) (func(string) bool, error) {
	return newMatcherMode(pattern, engine, true)
}

func newMatcherMode(pattern, engine string, pathAware bool) (func(string) bool, error) {
	var expression string
	switch engine {
	case "glob":
		var err error
		expression, err = globExpression(pattern, pathAware)
		if err != nil {
			return nil, err
		}
	case "regex":
		expression = pattern
	default:
		return nil, fmt.Errorf("unsupported search engine %q", engine)
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return nil, err
	}
	return compiled.MatchString, nil
}

func globExpression(pattern string, pathAware bool) (string, error) {
	var expression strings.Builder
	expression.WriteString("(?s)^")
	for index := 0; index < len(pattern); {
		switch pattern[index] {
		case '*':
			if !pathAware {
				for index < len(pattern) && pattern[index] == '*' {
					index++
				}
				expression.WriteString(".*")
				continue
			}
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index += 2
				if index < len(pattern) && pattern[index] == '/' {
					expression.WriteString("(?:.*/)?")
					index++
				} else {
					expression.WriteString(".*")
				}
				continue
			}
			expression.WriteString("[^/]*")
			index++
		case '?':
			if pathAware {
				expression.WriteString("[^/]")
			} else {
				expression.WriteByte('.')
			}
			index++
		case '[':
			class, next, err := globCharacterClass(pattern, index)
			if err != nil {
				return "", err
			}
			expression.WriteString(class)
			index = next
		case '\\':
			if index+1 >= len(pattern) {
				return "", fmt.Errorf("trailing escape in glob pattern")
			}
			expression.WriteString(regexp.QuoteMeta(pattern[index+1 : index+2]))
			index += 2
		default:
			start := index
			for index < len(pattern) && !strings.ContainsRune("*?[\\", rune(pattern[index])) {
				index++
			}
			expression.WriteString(regexp.QuoteMeta(pattern[start:index]))
		}
	}
	expression.WriteByte('$')
	return expression.String(), nil
}

func globCharacterClass(pattern string, start int) (string, int, error) {
	end := start + 1
	escaped := false
	for ; end < len(pattern); end++ {
		if escaped {
			escaped = false
			continue
		}
		if pattern[end] == '\\' {
			escaped = true
			continue
		}
		if pattern[end] == ']' {
			break
		}
	}
	if end >= len(pattern) || end == start+1 {
		return "", 0, fmt.Errorf("unterminated or empty character class in glob pattern")
	}
	content := pattern[start+1 : end]
	var class strings.Builder
	class.WriteByte('[')
	if strings.HasPrefix(content, "!") {
		class.WriteByte('^')
		content = content[1:]
		if content == "" {
			return "", 0, fmt.Errorf("empty negated character class in glob pattern")
		}
	} else if strings.HasPrefix(content, "^") {
		class.WriteString("\\^")
		content = content[1:]
	}
	class.WriteString(content)
	class.WriteByte(']')
	return class.String(), end + 1, nil
}

func changedLines(diff string) string {
	var result strings.Builder
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			inHunk = false
			continue
		}
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			result.WriteString(line[1:])
			result.WriteByte('\n')
		}
	}
	return result.String()
}

func searchGitError(action string, err error) error {
	return apperr.Wrap("git_history_error", fmt.Sprintf("failed to %s", action), apperr.ExitFailure, err, nil)
}
