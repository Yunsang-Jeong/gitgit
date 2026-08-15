package vscodehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yunsang/gitgit/internal/gitexec"
)

const (
	defaultHistoryLimit   = 100
	maxHistoryLimit       = 500
	maxBlameLines         = 2_000
	maxBlameMetadataBytes = 2 * 1024 * 1024
	maxTextBytes          = 4 * 1024 * 1024
)

var offlineGitEnvironment = []string{
	"GIT_NO_LAZY_FETCH=1",
	"GIT_TERMINAL_PROMPT=0",
	"GIT_OPTIONAL_LOCKS=0",
}

func (s *Server) listRefs(ctx context.Context, rawParams json.RawMessage) (RefsListResult, *RPCError) {
	var params RefsListParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return RefsListResult{}, rpcError(codeInvalidParams, "invalid_refs_params", err.Error())
	}
	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return RefsListResult{}, responseError
	}

	currentRef := ""
	if output, err := runReadOnlyGit(ctx, repository, nil, "symbolic-ref", "--quiet", "HEAD"); err == nil {
		currentRef = strings.TrimSpace(string(output))
	} else if ctx.Err() != nil {
		return RefsListResult{}, cancelledError()
	}

	output, err := runReadOnlyGit(
		ctx,
		repository,
		nil,
		"for-each-ref",
		"--format=%(refname)%00%(refname:short)%00%(objectname)%00%(symref)",
		"refs/heads",
		"refs/remotes",
	)
	if err != nil {
		return RefsListResult{}, gitCommandError(ctx, err)
	}
	if len(output) > MaxOutputBytes {
		return RefsListResult{}, rpcError(codeInternalError, "output_too_large", "Git ref output exceeds maxOutputBytes")
	}

	refs, parseError := parseRefs(output, currentRef)
	if parseError != nil {
		return RefsListResult{}, rpcError(codeInternalError, "invalid_git_output", parseError.Error())
	}
	return RefsListResult{Refs: refs}, nil
}

func parseRefs(output []byte, currentRef string) ([]Ref, error) {
	output = bytes.TrimSuffix(output, []byte{'\n'})
	if len(output) == 0 {
		return []Ref{}, nil
	}
	lines := bytes.Split(output, []byte{'\n'})
	refs := make([]Ref, 0, len(lines))
	for _, line := range lines {
		fields := bytes.Split(line, []byte{0})
		if len(fields) != 4 {
			return nil, fmt.Errorf("unexpected ref field count %d", len(fields))
		}
		fullName := string(fields[0])
		kind := ""
		switch {
		case strings.HasPrefix(fullName, "refs/heads/"):
			kind = "local"
		case strings.HasPrefix(fullName, "refs/remotes/"):
			kind = "remote"
		default:
			return nil, fmt.Errorf("unexpected ref %q", fullName)
		}
		refs = append(refs, Ref{
			Ref:        fullName,
			FullName:   fullName,
			Name:       string(fields[1]),
			Kind:       kind,
			Commit:     string(fields[2]),
			Current:    fullName == currentRef,
			SymbolicTo: string(fields[3]),
		})
	}
	return refs, nil
}

func (s *Server) blameLines(ctx context.Context, rawParams json.RawMessage) (BlameLinesResult, *RPCError) {
	var params BlameLinesParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return BlameLinesResult{}, rpcError(codeInvalidParams, "invalid_blame_params", err.Error())
	}
	if params.StartLine < 1 || params.EndLine < params.StartLine || params.EndLine-params.StartLine+1 > maxBlameLines {
		return BlameLinesResult{}, rpcError(
			codeInvalidParams,
			"invalid_line_range",
			fmt.Sprintf("line range must be 1-based, inclusive, and at most %d lines", maxBlameLines),
		)
	}

	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return BlameLinesResult{}, responseError
	}
	relativePath, responseError := validateRelativePath(repository.Root, params.path())
	if responseError != nil {
		return BlameLinesResult{}, responseError
	}

	resolvedRevision := ""
	if strings.TrimSpace(params.Revision) != "" {
		resolvedRevision, responseError = resolveRevision(ctx, repository, params.Revision)
		if responseError != nil {
			return BlameLinesResult{}, responseError
		}
		if _, responseError = ensureRevisionFile(ctx, repository, resolvedRevision, relativePath); responseError != nil {
			return BlameLinesResult{}, responseError
		}
	} else {
		if _, err := runReadOnlyGit(ctx, repository, nil, "ls-files", "--error-unmatch", "--", literalPathspec(relativePath)); err != nil {
			if ctx.Err() != nil {
				return BlameLinesResult{}, cancelledError()
			}
			return BlameLinesResult{}, rpcError(codeInvalidParams, "untracked_file", "relativePath must name a tracked file")
		}
	}

	var source []byte
	if params.Contents != nil {
		source = []byte(*params.Contents)
	} else if resolvedRevision != "" {
		source, responseError = readRevisionBlob(ctx, repository, resolvedRevision, relativePath)
		if responseError != nil {
			return BlameLinesResult{}, responseError
		}
	} else {
		source, responseError = readWorkingTreeFile(repository.Root, relativePath)
		if responseError != nil {
			return BlameLinesResult{}, responseError
		}
	}
	if responseError = validateText(source); responseError != nil {
		return BlameLinesResult{}, responseError
	}
	if params.EndLine > textLineCount(source) {
		return BlameLinesResult{}, rpcError(codeInvalidParams, "invalid_line_range", "endLine exceeds the file line count")
	}

	args := []string{
		"blame",
		"--line-porcelain",
		"-L", fmt.Sprintf("%d,%d", params.StartLine, params.EndLine),
	}
	if params.IgnoreWhitespace {
		args = append(args, "-w")
	}
	var stdin io.Reader
	if params.Contents != nil {
		args = append(args, "--contents", "-")
		stdin = strings.NewReader(*params.Contents)
	}
	if resolvedRevision != "" {
		args = append(args, resolvedRevision)
	}
	args = append(args, "--", relativePath)
	output, err := runReadOnlyGit(ctx, repository, stdin, args...)
	if err != nil {
		if ctx.Err() != nil {
			return BlameLinesResult{}, cancelledError()
		}
		return BlameLinesResult{}, rpcError(codeInternalError, "blame_failed", err.Error())
	}
	if len(output) > MaxOutputBytes {
		return BlameLinesResult{}, rpcError(codeInternalError, "output_too_large", "Git blame output exceeds maxOutputBytes")
	}
	lines, err := parseLinePorcelain(output)
	if err != nil {
		return BlameLinesResult{}, rpcError(codeInternalError, "invalid_git_output", err.Error())
	}
	if len(lines) != params.EndLine-params.StartLine+1 {
		return BlameLinesResult{}, rpcError(codeInternalError, "invalid_git_output", "Git blame returned an unexpected line count")
	}
	commitMetadata, responseError := readBlameCommitMetadata(ctx, repository, lines)
	if responseError != nil {
		return BlameLinesResult{}, responseError
	}
	return BlameLinesResult{Lines: lines, CommitMetadata: commitMetadata}, nil
}

func readBlameCommitMetadata(
	ctx context.Context,
	repository *gitexec.Repository,
	lines []BlameLine,
) (map[string]BlameCommitMetadata, *RPCError) {
	unique := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		if seen[line.Commit] || isWorkingTreeObjectID(line.Commit) {
			continue
		}
		seen[line.Commit] = true
		unique = append(unique, line.Commit)
	}
	if len(unique) == 0 {
		return map[string]BlameCommitMetadata{}, nil
	}

	input := strings.NewReader(strings.Join(unique, "\n") + "\n")
	output, err := repository.Runner.RunWithEnvLimit(
		ctx,
		repository.Root,
		input,
		offlineGitEnvironment,
		maxBlameMetadataBytes,
		"log",
		"-z",
		"--no-walk=unsorted",
		"--no-show-signature",
		"--format=%H%x00%P%x00%B",
		"--stdin",
	)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return nil, cancelledError()
		}
		return map[string]BlameCommitMetadata{}, nil
	}
	fields := bytes.Split(output, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%3 != 0 {
		return map[string]BlameCommitMetadata{}, nil
	}
	metadata := make(map[string]BlameCommitMetadata, len(unique))
	for index := 0; index < len(fields); index += 3 {
		commit := string(fields[index])
		if !isObjectID(commit) {
			return map[string]BlameCommitMetadata{}, nil
		}
		parents := strings.Fields(string(fields[index+1]))
		for _, parent := range parents {
			if !isObjectID(parent) {
				return map[string]BlameCommitMetadata{}, nil
			}
		}
		// Review inference only needs ordinary merge metadata. Keep an unusual
		// octopus commit from turning optional review metadata into a blame
		// response failure at the client protocol boundary.
		if len(parents) > 64 {
			continue
		}
		metadata[commit] = BlameCommitMetadata{
			Message:     string(fields[index+2]),
			ParentCount: len(parents),
		}
	}
	return metadata, nil
}

func isWorkingTreeObjectID(value string) bool {
	return (len(value) == 40 || len(value) == 64) && strings.Trim(value, "0") == ""
}

func parseLinePorcelain(output []byte) ([]BlameLine, error) {
	lines := bytes.Split(output, []byte{'\n'})
	result := make([]BlameLine, 0)
	for index := 0; index < len(lines); {
		if len(lines[index]) == 0 {
			index++
			continue
		}
		header := strings.Fields(string(lines[index]))
		index++
		if len(header) < 3 || !isObjectID(header[0]) {
			return nil, fmt.Errorf("invalid blame header")
		}
		originalLine, err := strconv.Atoi(header[1])
		if err != nil {
			return nil, fmt.Errorf("invalid original blame line: %w", err)
		}
		finalLine, err := strconv.Atoi(header[2])
		if err != nil {
			return nil, fmt.Errorf("invalid final blame line: %w", err)
		}

		item := BlameLine{Commit: header[0], OriginalLine: originalLine, Line: finalLine}
		authorTime := ""
		authorTimezone := ""
		foundContent := false
		for index < len(lines) {
			line := lines[index]
			index++
			if len(line) > 0 && line[0] == '\t' {
				item.Content = string(line[1:])
				foundContent = true
				break
			}
			key, value, ok := strings.Cut(string(line), " ")
			if !ok {
				continue
			}
			switch key {
			case "author":
				item.Author.Name = value
			case "author-mail":
				item.Author.Email = strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")
			case "author-time":
				authorTime = value
			case "author-tz":
				authorTimezone = value
			case "summary":
				item.Message = value
			}
		}
		if !foundContent {
			return nil, errors.New("blame record is missing content")
		}
		item.Date = formatGitTimestamp(authorTime, authorTimezone)
		result = append(result, item)
	}
	return result, nil
}

func formatGitTimestamp(seconds, timezone string) string {
	unixSeconds, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return ""
	}
	offset := 0
	if len(timezone) == 5 && (timezone[0] == '+' || timezone[0] == '-') {
		hours, hoursError := strconv.Atoi(timezone[1:3])
		minutes, minutesError := strconv.Atoi(timezone[3:5])
		if hoursError == nil && minutesError == nil && hours <= 23 && minutes <= 59 {
			offset = hours*60*60 + minutes*60
			if timezone[0] == '-' {
				offset = -offset
			}
		}
	}
	return time.Unix(unixSeconds, 0).In(time.FixedZone("", offset)).Format(time.RFC3339)
}

func (s *Server) fileHistory(ctx context.Context, rawParams json.RawMessage) (HistoryFileResult, *RPCError) {
	var params HistoryFileParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return HistoryFileResult{}, rpcError(codeInvalidParams, "invalid_history_params", err.Error())
	}
	if params.Limit < 0 {
		return HistoryFileResult{}, rpcError(codeInvalidParams, "invalid_limit", "limit must be zero or greater")
	}
	limit := params.Limit
	if limit == 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return HistoryFileResult{}, responseError
	}
	relativePath, responseError := validateRelativePath(repository.Root, params.path())
	if responseError != nil {
		return HistoryFileResult{}, responseError
	}
	revision := params.Revision
	if strings.TrimSpace(revision) == "" {
		revision = "HEAD"
	}
	resolvedRevision, responseError := resolveRevision(ctx, repository, revision)
	if responseError != nil {
		return HistoryFileResult{}, responseError
	}

	args := []string{
		"log",
		"--follow",
		"--find-renames",
		"--date-order",
		"-z",
		"--date=iso-strict",
		"--format=%H%x00%h%x00%P%x00%an%x00%ae%x00%aI%x00%B",
		fmt.Sprintf("--max-count=%d", limit),
		"--end-of-options",
		resolvedRevision,
		"--",
		relativePath,
	}
	output, err := runReadOnlyGit(ctx, repository, nil, args...)
	if err != nil {
		return HistoryFileResult{}, gitCommandError(ctx, err)
	}
	if len(output) > MaxOutputBytes {
		return HistoryFileResult{}, rpcError(codeInternalError, "output_too_large", "Git history output exceeds maxOutputBytes")
	}
	commits, err := parseFileHistory(output)
	if err != nil {
		return HistoryFileResult{}, rpcError(codeInternalError, "invalid_git_output", err.Error())
	}
	commitIDs := make([]string, len(commits))
	for index := range commits {
		commitIDs[index] = commits[index].Commit
	}
	filesByCommit, responseError := readCommitFilesBatch(ctx, repository, commitIDs)
	if responseError != nil {
		return HistoryFileResult{}, responseError
	}
	for index := range commits {
		commits[index].Files = filesByCommit[commits[index].Commit]
	}
	return HistoryFileResult{Commits: commits}, nil
}

func parseFileHistory(output []byte) ([]HistoryCommit, error) {
	fields := bytes.Split(output, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%7 != 0 {
		return nil, fmt.Errorf("unexpected history field count %d", len(fields))
	}
	commits := make([]HistoryCommit, 0, len(fields)/7)
	for index := 0; index < len(fields); index += 7 {
		commits = append(commits, HistoryCommit{
			Commit:      string(fields[index]),
			ShortCommit: string(fields[index+1]),
			Parents:     strings.Fields(string(fields[index+2])),
			Author: Author{
				Name:  string(fields[index+3]),
				Email: string(fields[index+4]),
			},
			Date:    string(fields[index+5]),
			Message: strings.TrimSpace(string(fields[index+6])),
			Files:   []FileChange{},
		})
	}
	return commits, nil
}

func readCommitFilesBatch(
	ctx context.Context,
	repository *gitexec.Repository,
	commits []string,
) (map[string][]FileChange, *RPCError) {
	result := make(map[string][]FileChange, len(commits))
	if len(commits) == 0 {
		return result, nil
	}
	for _, commit := range commits {
		if !isObjectID(commit) {
			return nil, rpcError(codeInternalError, "invalid_git_output", "history contained an invalid commit id")
		}
		result[commit] = []FileChange{}
	}
	firstParents, responseError := readFirstParentsBatch(ctx, repository, commits)
	if responseError != nil {
		return nil, responseError
	}

	var input strings.Builder
	for _, commit := range commits {
		parent := firstParents[commit]
		if parent == "" {
			input.WriteString(commit)
		} else {
			input.WriteString(commit)
			input.WriteByte(' ')
			input.WriteString(parent)
		}
		input.WriteByte('\n')
	}
	output, err := runReadOnlyGit(
		ctx,
		repository,
		strings.NewReader(input.String()),
		"diff-tree", "--stdin", "--root", "--name-status", "-r", "-z", "-M",
	)
	if err != nil {
		return nil, gitCommandError(ctx, err)
	}
	if len(output) > MaxOutputBytes {
		return nil, rpcError(codeInternalError, "output_too_large", "Git changed-file output exceeds maxOutputBytes")
	}

	tokens := bytes.Split(output, []byte{0})
	currentCommit := ""
	for index := 0; index < len(tokens); {
		token := string(tokens[index])
		index++
		if token == "" {
			continue
		}
		if commit, ok := parseDiffTreeCommit(token); ok {
			if _, expected := result[commit]; !expected {
				return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned an unexpected commit id")
			}
			currentCommit = commit
			continue
		}
		if currentCommit == "" || !validFileStatus(token) {
			return nil, rpcError(codeInternalError, "invalid_git_output", "invalid changed-file record")
		}
		if index >= len(tokens) {
			return nil, rpcError(codeInternalError, "invalid_git_output", "changed-file path is missing")
		}
		if token[0] == 'R' || token[0] == 'C' {
			if index+1 >= len(tokens) {
				return nil, rpcError(codeInternalError, "invalid_git_output", "rename paths are missing")
			}
			result[currentCommit] = append(result[currentCommit], FileChange{
				Status:  token,
				OldPath: string(tokens[index]),
				Path:    string(tokens[index+1]),
			})
			index += 2
			continue
		}
		result[currentCommit] = append(result[currentCommit], FileChange{Status: token, Path: string(tokens[index])})
		index++
	}
	return result, nil
}

func readFirstParentsBatch(
	ctx context.Context,
	repository *gitexec.Repository,
	commits []string,
) (map[string]string, *RPCError) {
	parents := make(map[string]string, len(commits))
	if len(commits) == 0 {
		return parents, nil
	}
	var input strings.Builder
	for _, commit := range commits {
		input.WriteString(commit)
		input.WriteByte('\n')
		parents[commit] = ""
	}
	output, err := runReadOnlyGit(
		ctx,
		repository,
		strings.NewReader(input.String()),
		"rev-list", "--parents", "--stdin", "--no-walk=unsorted",
	)
	if err != nil {
		return nil, gitCommandError(ctx, err)
	}
	if len(output) > MaxOutputBytes {
		return nil, rpcError(codeInternalError, "output_too_large", "Git parent output exceeds maxOutputBytes")
	}
	seen := make(map[string]bool, len(commits))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		commit := fields[0]
		if _, expected := parents[commit]; !expected || seen[commit] {
			return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned unexpected parent metadata")
		}
		seen[commit] = true
		if len(fields) > 1 {
			if !isObjectID(fields[1]) {
				return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned an invalid parent id")
			}
			parents[commit] = fields[1]
		}
		for index := 2; index < len(fields); index++ {
			parent := fields[index]
			if !isObjectID(parent) {
				return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned an invalid parent id")
			}
		}
	}
	for commit := range parents {
		if !seen[commit] {
			return nil, rpcError(codeInternalError, "invalid_git_output", "Git omitted requested parent metadata")
		}
	}
	return parents, nil
}

func parseDiffTreeCommit(value string) (string, bool) {
	if isObjectID(value) {
		return value, true
	}
	return "", false
}

func validFileStatus(value string) bool {
	if value == "" || !strings.ContainsRune("ACDMRTUXB", rune(value[0])) {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (s *Server) revisionContent(ctx context.Context, rawParams json.RawMessage) (RevisionContentResult, *RPCError) {
	var params RevisionContentParams
	if err := decodeObjectParams(rawParams, &params); err != nil {
		return RevisionContentResult{}, rpcError(codeInvalidParams, "invalid_revision_content_params", err.Error())
	}
	repository, responseError := s.openRepository(ctx, params.root())
	if responseError != nil {
		return RevisionContentResult{}, responseError
	}
	relativePath, responseError := validateRelativePath(repository.Root, params.path())
	if responseError != nil {
		return RevisionContentResult{}, responseError
	}
	resolvedRevision, responseError := resolveRevision(ctx, repository, params.Revision)
	if responseError != nil {
		return RevisionContentResult{}, responseError
	}
	content, responseError := readRevisionBlob(ctx, repository, resolvedRevision, relativePath)
	if responseError != nil {
		return RevisionContentResult{}, responseError
	}
	return RevisionContentResult{Content: string(content)}, nil
}

func decodeObjectParams(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("params must be an object")
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return errors.New("params must be an object")
	}
	return nil
}

func (s *Server) openRepository(ctx context.Context, root string) (*gitexec.Repository, *RPCError) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, rpcError(codeInvalidParams, "repository_root_required", "repositoryRoot is required")
	}
	if !filepath.IsAbs(root) {
		return nil, rpcError(codeInvalidParams, "invalid_repository_root", "repositoryRoot must be absolute")
	}
	resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return nil, rpcError(codeInvalidParams, "invalid_repository_root", "repositoryRoot must exist")
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil || !info.IsDir() {
		return nil, rpcError(codeInvalidParams, "invalid_repository_root", "repositoryRoot must be an existing directory")
	}

	installation, err := s.resolveGit(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, cancelledError()
		}
		return nil, rpcError(codeGitNotFound, "git_not_found", err.Error())
	}
	repository, err := gitexec.OpenRepository(ctx, &gitexec.Runner{Binary: installation.Path}, resolvedRoot)
	if err != nil {
		if ctx.Err() != nil {
			return nil, cancelledError()
		}
		return nil, rpcError(codeNotGitRepository, "not_git_repository", err.Error())
	}
	canonicalRoot := canonicalPath(repository.Root)
	if canonicalRoot != canonicalPath(resolvedRoot) {
		return nil, rpcError(
			codeInvalidParams,
			"repository_root_mismatch",
			"repositoryRoot must equal the discovered worktree root",
		)
	}
	repository.Root = canonicalRoot
	return repository, nil
}

func validateRelativePath(root, candidate string) (string, *RPCError) {
	if strings.TrimSpace(candidate) == "" {
		return "", rpcError(codeInvalidParams, "relative_path_required", "relativePath is required")
	}
	if strings.ContainsRune(candidate, 0) || strings.ContainsRune(candidate, '\n') || strings.ContainsRune(candidate, '\r') {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath contains an invalid control character")
	}
	if filepath.IsAbs(candidate) || pathpkg.IsAbs(candidate) || strings.Contains(candidate, `\`) || windowsDrivePath(candidate) {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath must be a portable repository-relative path")
	}
	for _, component := range strings.Split(candidate, "/") {
		if component == "" || component == "." || component == ".." {
			return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath contains an invalid component")
		}
	}
	cleaned := pathpkg.Clean(candidate)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath cannot escape the repository")
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git/") {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath cannot address Git metadata")
	}

	joined := filepath.Join(root, filepath.FromSlash(cleaned))
	if !pathWithin(root, joined) {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath cannot escape the repository")
	}
	resolved, err := resolveExistingAncestor(joined)
	if err != nil || !pathWithin(root, resolved) {
		return "", rpcError(codeInvalidParams, "invalid_file_path", "relativePath resolves outside the repository")
	}
	return cleaned, nil
}

func windowsDrivePath(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	return (path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')
}

func resolveExistingAncestor(candidate string) (string, error) {
	current := filepath.Clean(candidate)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return resolved, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		current = parent
	}
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(canonicalPath(root), canonicalPath(candidate))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func literalPathspec(relativePath string) string {
	return ":(literal)" + relativePath
}

func validateRevision(revision string) *RPCError {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return rpcError(codeInvalidParams, "revision_required", "revision is required")
	}
	if strings.HasPrefix(revision, "-") {
		return rpcError(codeInvalidParams, "invalid_revision", "revision cannot be a Git option")
	}
	if strings.ContainsRune(revision, 0) || strings.ContainsRune(revision, '\n') || strings.ContainsRune(revision, '\r') {
		return rpcError(codeInvalidParams, "invalid_revision", "revision contains an invalid control character")
	}
	return nil
}

func resolveRevision(ctx context.Context, repository *gitexec.Repository, revision string) (string, *RPCError) {
	revision = strings.TrimSpace(revision)
	if responseError := validateRevision(revision); responseError != nil {
		return "", responseError
	}
	output, err := runReadOnlyGit(ctx, repository, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return "", cancelledError()
		}
		return "", rpcError(codeInvalidParams, "invalid_revision", "revision does not resolve to a commit")
	}
	resolved := strings.TrimSpace(string(output))
	if !isObjectID(resolved) {
		return "", rpcError(codeInternalError, "invalid_git_output", "Git returned an invalid commit id")
	}
	return resolved, nil
}

func ensureRevisionFile(
	ctx context.Context,
	repository *gitexec.Repository,
	revision string,
	relativePath string,
) (string, *RPCError) {
	objectSpec := revision + ":" + relativePath
	if _, err := runReadOnlyGit(ctx, repository, nil, "cat-file", "-e", objectSpec); err != nil {
		if ctx.Err() != nil {
			return "", cancelledError()
		}
		return "", rpcError(codeInvalidParams, "revision_content_not_found", "file does not exist at the requested revision")
	}
	return objectSpec, nil
}

func readRevisionBlob(
	ctx context.Context,
	repository *gitexec.Repository,
	revision string,
	relativePath string,
) ([]byte, *RPCError) {
	objectSpec, responseError := ensureRevisionFile(ctx, repository, revision, relativePath)
	if responseError != nil {
		return nil, responseError
	}
	typeOutput, err := runReadOnlyGit(ctx, repository, nil, "cat-file", "-t", objectSpec)
	if err != nil {
		return nil, gitCommandError(ctx, err)
	}
	if strings.TrimSpace(string(typeOutput)) != "blob" {
		return nil, rpcError(
			codeInvalidParams,
			"revision_content_not_file",
			"path at the requested revision is not a file",
		)
	}
	sizeOutput, err := runReadOnlyGit(ctx, repository, nil, "cat-file", "-s", objectSpec)
	if err != nil {
		return nil, gitCommandError(ctx, err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 0 {
		return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned an invalid blob size")
	}
	if size > maxTextBytes {
		return nil, rpcError(codeInvalidParams, "file_too_large", "file exceeds the helper text size limit")
	}
	output, err := runReadOnlyGit(ctx, repository, nil, "cat-file", "blob", objectSpec)
	if err != nil {
		return nil, gitCommandError(ctx, err)
	}
	if int64(len(output)) != size {
		return nil, rpcError(codeInternalError, "invalid_git_output", "Git returned an unexpected blob size")
	}
	if responseError := validateText(output); responseError != nil {
		return nil, responseError
	}
	return output, nil
}

func readWorkingTreeFile(root, relativePath string) ([]byte, *RPCError) {
	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Stat(fullPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, rpcError(codeInvalidParams, "file_not_found", "relativePath must name an existing regular file")
	}
	if info.Size() > maxTextBytes {
		return nil, rpcError(codeInvalidParams, "file_too_large", "file exceeds the helper text size limit")
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, rpcError(codeInternalError, "file_read_failed", err.Error())
	}
	return content, nil
}

func validateText(content []byte) *RPCError {
	if len(content) > maxTextBytes {
		return rpcError(codeInvalidParams, "file_too_large", "file exceeds the helper text size limit")
	}
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return rpcError(codeInvalidParams, "binary_file", "binary or non-UTF-8 files are not supported")
	}
	return nil
}

func textLineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	count := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		count++
	}
	return count
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func runReadOnlyGit(
	ctx context.Context,
	repository *gitexec.Repository,
	stdin io.Reader,
	args ...string,
) ([]byte, error) {
	return repository.Runner.RunWithEnv(ctx, repository.Root, stdin, offlineGitEnvironment, args...)
}
