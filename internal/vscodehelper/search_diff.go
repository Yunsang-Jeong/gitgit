package vscodehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

const (
	defaultSearchLimit = 100
	maxSearchLimit     = 250
	maxSearchPatterns  = 64
	maxPatternBytes    = 16 * 1024
	defaultDiffContext = 3
	maxDiffContext     = 100
	maxProgressUpdates = 200
)

type progressReporter struct {
	requestID json.RawMessage
	sink      *responseSink

	mu       sync.Mutex
	sent     int
	last     app.SearchProgress
	finished bool
}

func newProgressReporter(requestID json.RawMessage, sink *responseSink) *progressReporter {
	return &progressReporter{
		requestID: append(json.RawMessage(nil), requestID...),
		sink:      sink,
	}
}

func (p *progressReporter) report(progress app.SearchProgress) {
	if p == nil || p.sink == nil || len(p.requestID) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.last = progress
	if p.sent >= maxProgressUpdates {
		return
	}
	p.sent++
	_ = p.sink.writeNotification(ProgressNotification{
		JSONRPC: "2.0",
		Method:  "$/progress",
		Params: ProgressParams{
			RequestID: append(json.RawMessage(nil), p.requestID...),
			Scanned:   progress.Scanned,
			Total:     progress.Total,
		},
	})
}

func (p *progressReporter) done(scanned int, cancelled bool) {
	if p == nil || p.sink == nil || len(p.requestID) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.finished = true
	if scanned == 0 {
		scanned = p.last.Scanned
	}
	total := p.last.Total
	if total < scanned {
		total = scanned
	}
	_ = p.sink.writeNotification(ProgressNotification{
		JSONRPC: "2.0",
		Method:  "$/progress",
		Params: ProgressParams{
			RequestID: append(json.RawMessage(nil), p.requestID...),
			Scanned:   scanned,
			Total:     total,
			Done:      true,
			Cancelled: cancelled,
		},
	})
}

func (s *Server) searchRun(
	ctx context.Context,
	rawParams json.RawMessage,
	progress *progressReporter,
) (SearchRunResult, *RPCError) {
	var params SearchRunParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_search_params", err.Error())
	}
	if len(params.Patterns) == 0 {
		return SearchRunResult{}, rpcError(codeInvalidParams, "missing_search_pattern", "patterns must contain at least one condition")
	}
	if len(params.Patterns) > maxSearchPatterns {
		return SearchRunResult{}, rpcError(
			codeInvalidParams,
			"too_many_search_patterns",
			fmt.Sprintf("patterns cannot contain more than %d conditions", maxSearchPatterns),
		)
	}
	if params.Limit < 0 {
		return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_limit", "limit must be zero or greater")
	}
	limit := params.Limit
	if limit == 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	if params.Context < 0 || params.Context > maxDiffContext {
		return SearchRunResult{}, rpcError(
			codeInvalidParams,
			"invalid_context",
			fmt.Sprintf("context must be between zero and %d", maxDiffContext),
		)
	}
	contextLines := params.Context
	if contextLines == 0 {
		contextLines = defaultDiffContext
	}

	engine := strings.ToLower(strings.TrimSpace(params.Engine))
	if engine == "" {
		engine = "glob"
	}
	if engine != "glob" && engine != "regex" {
		return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_engine", "engine must be glob or regex")
	}
	for _, filter := range []struct {
		name  string
		value string
	}{
		{name: "author", value: params.Author},
		{name: "since", value: params.Since},
		{name: "until", value: params.Until},
	} {
		if strings.ContainsRune(filter.value, 0) || strings.ContainsRune(filter.value, '\n') || strings.ContainsRune(filter.value, '\r') {
			return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_search_filter", filter.name+" contains an invalid control character")
		}
	}

	predicates := make([]app.SearchPredicate, 0, len(params.Patterns))
	for index, pattern := range params.Patterns {
		source := strings.ToLower(strings.TrimSpace(pattern.Source))
		if source != "msg" && source != "diff" && source != "file" {
			return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_pattern_source", fmt.Sprintf("unsupported pattern source %q", pattern.Source))
		}
		value := strings.TrimSpace(pattern.Value)
		if value == "" {
			return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_pattern", fmt.Sprintf("pattern %d is empty", index+1))
		}
		if len(value) > maxPatternBytes {
			return SearchRunResult{}, rpcError(codeInvalidParams, "pattern_too_large", fmt.Sprintf("pattern %d exceeds the size limit", index+1))
		}
		join := strings.ToLower(strings.TrimSpace(pattern.Join))
		if index == 0 {
			join = ""
		} else if join == "" {
			join = "or"
		} else if join != "and" && join != "or" {
			return SearchRunResult{}, rpcError(codeInvalidParams, "invalid_search_join", fmt.Sprintf("unsupported search join %q", pattern.Join))
		}
		predicates = append(predicates, app.SearchPredicate{
			Source:      source,
			Value:       value,
			Join:        join,
			OpenGroups:  pattern.openGroups(),
			CloseGroups: pattern.closeGroups(),
		})
	}

	allRefs := params.allRefs()
	scope := strings.TrimSpace(params.Scope)
	options := app.SearchOptions{
		Predicates:     predicates,
		Engine:         engine,
		All:            allRefs,
		Author:         strings.TrimSpace(params.Author),
		Since:          strings.TrimSpace(params.Since),
		Until:          strings.TrimSpace(params.Until),
		FollowRename:   params.followRename(),
		Limit:          limit,
		Context:        contextLines,
		OmitResultDiff: true,
	}
	if allRefs {
		scope = "All refs"
	} else {
		if scope == "" {
			scope = "HEAD"
		}
		if responseError := validateRevision(scope); responseError != nil {
			return SearchRunResult{}, responseError
		}
		if scope != "HEAD" {
			options.Revision = scope
		}
	}

	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return SearchRunResult{}, responseError
	}
	searchResponse, err := app.NewSearchService(repository).SearchWithProgress(ctx, options, progress.report)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return SearchRunResult{}, cancelledError()
		}
		stableCode, message, _ := apperr.Details(err)
		if apperr.ExitCode(err) == apperr.ExitUsage {
			return SearchRunResult{}, rpcError(codeInvalidParams, stableCode, message)
		}
		return SearchRunResult{}, rpcError(codeInternalError, stableCode, message)
	}
	if ctx.Err() != nil {
		return SearchRunResult{}, cancelledError()
	}

	results := make([]SearchResult, 0, len(searchResponse.Results))
	for _, match := range searchResponse.Results {
		files := make([]FileChange, 0, len(match.Files))
		for _, file := range match.Files {
			files = append(files, FileChange{Status: file.Status, OldPath: file.OldPath, Path: file.Path})
		}
		results = append(results, SearchResult{
			Author:      Author{Name: match.Author.Name, Email: match.Author.Email},
			Commit:      match.Commit,
			ShortCommit: shortObjectID(match.Commit),
			Message:     match.Message,
			Date:        match.Date,
			Refs:        append([]string(nil), match.Refs...),
			File: FileChange{
				Status:  match.File.Status,
				OldPath: match.File.OldPath,
				Path:    match.File.Path,
			},
			Files:        files,
			Diff:         match.Diff,
			MatchSources: append([]string(nil), match.MatchSources...),
		})
	}
	return SearchRunResult{
		Scope:   scope,
		AllRefs: allRefs,
		Scanned: searchResponse.Scanned,
		Count:   len(results),
		Results: results,
	}, nil
}

func shortObjectID(objectID string) string {
	if len(objectID) <= 8 {
		return objectID
	}
	return objectID[:8]
}

func (s *Server) diffFile(ctx context.Context, rawParams json.RawMessage) (DiffFileResult, *RPCError) {
	var params DiffFileParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return DiffFileResult{}, rpcError(codeInvalidParams, "invalid_diff_params", err.Error())
	}
	if params.Context < 0 || params.Context > maxDiffContext {
		return DiffFileResult{}, rpcError(
			codeInvalidParams,
			"invalid_context",
			fmt.Sprintf("context must be between zero and %d", maxDiffContext),
		)
	}
	contextLines := params.Context
	if contextLines == 0 {
		contextLines = defaultDiffContext
	}

	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return DiffFileResult{}, responseError
	}
	relativePath, responseError := validateRelativePath(repository.Root, params.Path)
	if responseError != nil {
		return DiffFileResult{}, responseError
	}
	requestedOldPath := params.oldPath()
	if requestedOldPath != "" {
		requestedOldPath, responseError = validateRelativePath(repository.Root, requestedOldPath)
		if responseError != nil {
			return DiffFileResult{}, responseError
		}
	}
	commit, responseError := resolveRevision(ctx, repository, params.Commit)
	if responseError != nil {
		return DiffFileResult{}, responseError
	}

	parent, responseError := firstParent(ctx, repository, commit)
	if responseError != nil {
		return DiffFileResult{}, responseError
	}
	filesByCommit, responseError := readCommitFilesBatch(ctx, repository, []string{commit})
	if responseError != nil {
		return DiffFileResult{}, responseError
	}
	change, ok := selectDiffChange(filesByCommit[commit], relativePath, requestedOldPath)
	if !ok {
		return DiffFileResult{}, rpcError(codeInvalidParams, "diff_not_found", "path was not changed by the requested commit")
	}

	paths := make([]string, 0, 2)
	if change.OldPath != "" {
		paths = append(paths, change.OldPath)
	}
	paths = append(paths, change.Path)
	args := []string{
		"show",
		"--format=",
		"--root",
		"--first-parent",
		"--no-ext-diff",
		"--no-color",
		"--find-renames",
		fmt.Sprintf("--unified=%d", contextLines),
		commit,
		"--",
	}
	args = append(args, paths...)
	output, err := runReadOnlyGit(ctx, repository, nil, args...)
	if err != nil {
		return DiffFileResult{}, gitCommandError(ctx, err)
	}
	if len(output) > maxTextBytes {
		return DiffFileResult{}, rpcError(codeInternalError, "output_too_large", "file diff exceeds the helper text size limit")
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return DiffFileResult{}, rpcError(codeInvalidParams, "diff_not_found", "Git returned no file diff")
	}
	if !utf8Text(output) {
		return DiffFileResult{}, rpcError(codeInvalidParams, "binary_file", "file diff is not UTF-8 text")
	}
	diff := strings.TrimRight(string(output), "\n")
	if binaryDiff(diff) {
		return DiffFileResult{
			Parent:          parent,
			Diff:            diff,
			TargetRanges:    []TargetRange{},
			DeletionAnchors: []DeletionAnchor{},
		}, nil
	}

	targetLineCount := 0
	if targetContent, targetError := readRevisionBlob(ctx, repository, commit, change.Path); targetError == nil {
		targetLineCount = textLineCount(targetContent)
	} else if targetError.Data.Code != "revision_content_not_found" {
		return DiffFileResult{}, targetError
	}
	targetRanges, deletionAnchors, err := parseUnifiedRanges(diff, targetLineCount)
	if err != nil {
		return DiffFileResult{}, rpcError(codeInternalError, "invalid_git_output", err.Error())
	}
	return DiffFileResult{
		Parent:          parent,
		Diff:            diff,
		TargetRanges:    targetRanges,
		DeletionAnchors: deletionAnchors,
	}, nil
}

func firstParent(ctx context.Context, repository *gitexec.Repository, commit string) (string, *RPCError) {
	output, err := runReadOnlyGit(ctx, repository, nil, "rev-list", "--parents", "--max-count=1", "--end-of-options", commit)
	if err != nil {
		return "", gitCommandError(ctx, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 || fields[0] != commit {
		return "", rpcError(codeInternalError, "invalid_git_output", "Git returned invalid parent metadata")
	}
	if len(fields) == 1 {
		return "", nil
	}
	if !isObjectID(fields[1]) {
		return "", rpcError(codeInternalError, "invalid_git_output", "Git returned an invalid parent id")
	}
	return fields[1], nil
}

func selectDiffChange(changes []FileChange, path, oldPath string) (FileChange, bool) {
	for _, change := range changes {
		if change.Path != path {
			continue
		}
		if oldPath == "" || change.OldPath == oldPath {
			return change, true
		}
	}
	return FileChange{}, false
}

func binaryDiff(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch" {
			return true
		}
	}
	return false
}

func utf8Text(content []byte) bool {
	return bytes.IndexByte(content, 0) < 0 && utf8.Valid(content)
}

var unifiedHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func parseUnifiedRanges(diff string, targetLineCount int) ([]TargetRange, []DeletionAnchor, error) {
	lines := strings.Split(diff, "\n")
	ranges := make([]TargetRange, 0)
	anchors := make([]DeletionAnchor, 0)
	inHunk := false
	newLine := 0
	addStart := 0
	deletionActive := false

	flushAddition := func() {
		if addStart == 0 {
			return
		}
		ranges = appendRange(ranges, TargetRange{StartLine: addStart, EndLine: newLine - 1})
		addStart = 0
	}
	flushDeletion := func() {
		if !deletionActive {
			return
		}
		anchor := newLine
		if targetLineCount <= 0 {
			anchor = 1
		} else {
			if anchor < 1 {
				anchor = 1
			}
			if anchor > targetLineCount {
				anchor = targetLineCount
			}
		}
		if len(anchors) == 0 || anchors[len(anchors)-1].AnchorLine != anchor {
			anchors = append(anchors, DeletionAnchor{AnchorLine: anchor})
		}
		deletionActive = false
	}

	for _, line := range lines {
		if matches := unifiedHunkHeader.FindStringSubmatch(line); matches != nil {
			flushAddition()
			flushDeletion()
			parsed, err := strconv.Atoi(matches[3])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid target hunk line: %w", err)
			}
			newLine = parsed
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(line, "diff --git ") {
			flushAddition()
			flushDeletion()
			inHunk = false
			continue
		}
		if line == `\ No newline at end of file` {
			continue
		}
		if line == "" {
			// strings.Split adds an empty record for a final newline. A genuine
			// empty context line still has a leading space in unified diff.
			continue
		}
		switch line[0] {
		case ' ':
			flushAddition()
			flushDeletion()
			newLine++
		case '-':
			flushAddition()
			deletionActive = true
		case '+':
			flushDeletion()
			if addStart == 0 {
				addStart = newLine
			}
			newLine++
		default:
			flushAddition()
			flushDeletion()
			inHunk = false
		}
	}
	flushAddition()
	flushDeletion()
	return ranges, anchors, nil
}

func appendRange(ranges []TargetRange, candidate TargetRange) []TargetRange {
	if candidate.StartLine < 1 || candidate.EndLine < candidate.StartLine {
		return ranges
	}
	if len(ranges) > 0 && candidate.StartLine <= ranges[len(ranges)-1].EndLine+1 {
		if candidate.EndLine > ranges[len(ranges)-1].EndLine {
			ranges[len(ranges)-1].EndLine = candidate.EndLine
		}
		return ranges
	}
	return append(ranges, candidate)
}
