package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/gitexec"
)

const (
	maximumRewriteCommits        = 100
	maximumEditableFile          = 2 << 20
	maximumRewriteProvenanceNote = 500
	maximumRewriteAuthorName     = 256
	maximumRewriteAuthorEmail    = 320

	rewriteProvenanceTrailer       = "rewritten from:"
	legacyRewriteProvenanceTrailer = "GitGit-Rewritten-From:"
	rewriteProvenanceNoteTrailer   = "GitGit-Rewrite-Note:"
)

type CommitEditStack struct {
	Branch              string             `json:"branch"`
	DefaultBranch       string             `json:"default_branch"`
	Head                string             `json:"head"`
	Base                string             `json:"base"`
	DefaultBranchTarget bool               `json:"default_branch_target"`
	Commits             []CommitEditCommit `json:"commits"`
}

// CommitEditTarget is the safe, first-parent-only head range that the Commit
// Page may expose for local editing. It intentionally stops before a merge or
// root commit because RewriteCommits cannot replay across either boundary.
type CommitEditTarget struct {
	Branch  string          `json:"branch"`
	Commits []CommitSummary `json:"commits"`
}

type CommitEditCommit struct {
	Author      app.Author       `json:"author"`
	Commit      string           `json:"commit"`
	ShortCommit string           `json:"short_commit"`
	Message     string           `json:"message"`
	Date        string           `json:"date"`
	Files       []app.FileChange `json:"files"`
	Parents     []string         `json:"-"`
}

type CommitFileContent struct {
	Commit         string `json:"commit"`
	Path           string `json:"path"`
	Content        string `json:"content"`
	Exists         bool   `json:"exists"`
	Editable       bool   `json:"editable"`
	Restorable     bool   `json:"restorable"`
	RestoreContent string `json:"restore_content,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type CommitFileEdit struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Delete  bool   `json:"delete"`
}

type RewriteCommit struct {
	Commit     string           `json:"commit"`
	Message    string           `json:"message"`
	Author     *app.Author      `json:"author,omitempty"`
	AuthorDate *string          `json:"author_date,omitempty"`
	FileEdits  []CommitFileEdit `json:"file_edits,omitempty"`
}

type RewriteCommitsRequest struct {
	Branch                  string          `json:"branch"`
	ExpectedHead            string          `json:"expected_head"`
	Base                    string          `json:"base"`
	ConfirmDefaultBranch    bool            `json:"confirm_default_branch"`
	AppendRewriteProvenance bool            `json:"append_rewrite_provenance"`
	RewriteProvenanceNote   string          `json:"rewrite_provenance_note"`
	Commits                 []RewriteCommit `json:"commits"`
}

type RewriteCommitsResponse struct {
	State     RepositoryState `json:"state"`
	Head      string          `json:"head"`
	BackupRef string          `json:"backup_ref"`
	Warning   string          `json:"warning,omitempty"`
}

func (s *Service) PrepareCommitEdit(ctx context.Context, startOID string) (CommitEditStack, error) {
	return s.prepareCommitEdit(ctx, startOID, "")
}

// CommitEditTargetForBranch reads the editable first-parent head range without
// changing the active worktree. Callers still need the target branch checked
// out before they can prepare or apply a rewrite.
func (s *Service) CommitEditTargetForBranch(ctx context.Context, branch string) (CommitEditTarget, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return CommitEditTarget{}, err
	}
	return commitEditTargetForBranch(operationContext, repository, branch)
}

func commitEditTargetForBranch(ctx context.Context, repository *gitexec.Repository, branch string) (CommitEditTarget, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return CommitEditTarget{}, errors.New("select a local branch to edit")
	}
	if _, err := repository.Run(ctx, nil, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		return CommitEditTarget{}, fmt.Errorf("the target branch %q is not a local branch; open a local worktree for it before editing", branch)
	}
	output, err := repository.Run(ctx, nil,
		"rev-list", "--first-parent", fmt.Sprintf("--max-count=%d", maximumRewriteCommits), "--end-of-options", branch,
	)
	if err != nil {
		return CommitEditTarget{}, fmt.Errorf("read first-parent history for %s: %w", branch, err)
	}
	target := CommitEditTarget{Branch: branch, Commits: make([]CommitSummary, 0, maximumRewriteCommits)}
	for _, oid := range strings.Fields(string(output)) {
		summary, summaryErr := readCommitSummary(ctx, repository, oid)
		if summaryErr != nil {
			return CommitEditTarget{}, summaryErr
		}
		if len(summary.Parents) == 0 {
			break
		}
		if len(summary.Parents) > 1 {
			break
		}
		target.Commits = append(target.Commits, summary)
	}
	if len(target.Commits) == 0 {
		return CommitEditTarget{}, fmt.Errorf("the editable first-parent head range of %q is empty; GitGit cannot rewrite a merge or root commit", branch)
	}
	return target, nil
}

// PrepareCommitEditForBranch keeps the UI's displayed branch and the branch
// that will be rewritten on the same lease. The repository is never checked
// out here; callers must select the attached worktree before starting.
func (s *Service) PrepareCommitEditForBranch(ctx context.Context, startOID, expectedBranch string) (CommitEditStack, error) {
	return s.prepareCommitEdit(ctx, startOID, expectedBranch)
}

func (s *Service) prepareCommitEdit(ctx context.Context, startOID, expectedBranch string) (CommitEditStack, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return CommitEditStack{}, err
	}
	return prepareCommitEdit(operationContext, repository, strings.TrimSpace(startOID), strings.TrimSpace(expectedBranch))
}

func prepareCommitEdit(ctx context.Context, repository *gitexec.Repository, startOID, expectedBranch string) (CommitEditStack, error) {
	if startOID == "" {
		return CommitEditStack{}, errors.New("select a commit to start editing")
	}
	branch, err := currentLocalBranch(ctx, repository)
	if err != nil {
		return CommitEditStack{}, err
	}
	if expectedBranch != "" && expectedBranch != branch {
		return CommitEditStack{}, fmt.Errorf("the target branch is %q, but this worktree has %q checked out; select that branch's worktree before entering Edit Mode", expectedBranch, branch)
	}
	head, err := resolveCommit(ctx, repository, "HEAD")
	if err != nil {
		return CommitEditStack{}, fmt.Errorf("resolve current HEAD: %w", err)
	}
	start, err := resolveCommit(ctx, repository, startOID)
	if err != nil {
		return CommitEditStack{}, fmt.Errorf("resolve selected commit: %w", err)
	}
	chainOutput, err := repository.Run(ctx, nil, "rev-list", "--first-parent", head)
	if err != nil {
		return CommitEditStack{}, fmt.Errorf("read current branch history: %w", err)
	}
	newestFirst := strings.Fields(string(chainOutput))
	selectedIndex := -1
	for index, oid := range newestFirst {
		if oid == start {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 {
		containingBranches, branchesErr := readContainingBranches(ctx, repository, start)
		if branchesErr == nil && len(containingBranches) > 0 && !containsBranch(containingBranches, branch) {
			return CommitEditStack{}, fmt.Errorf(
				"the selected commit belongs to %s, but this worktree has %s checked out; GitGit rewrites only the checked-out branch, so open a worktree for that branch before editing",
				strings.Join(containingBranches, ", "), branch,
			)
		}
		return CommitEditStack{}, fmt.Errorf("the selected commit is not on the first-parent history of the checked-out branch %s; rewriting across a merge is not supported", branch)
	}
	if selectedIndex+1 > maximumRewriteCommits {
		return CommitEditStack{}, fmt.Errorf("the selected range contains %d commits; GitGit edits at most %d at once", selectedIndex+1, maximumRewriteCommits)
	}

	selectedSummary, err := readCommitSummary(ctx, repository, start)
	if err != nil {
		return CommitEditStack{}, err
	}
	if len(selectedSummary.Parents) == 0 {
		return CommitEditStack{}, errors.New("rewriting the root commit is not supported")
	}
	base := selectedSummary.Parents[0]
	commits := make([]CommitEditCommit, 0, selectedIndex+1)
	for index := selectedIndex; index >= 0; index-- {
		summary, summaryErr := readCommitSummary(ctx, repository, newestFirst[index])
		if summaryErr != nil {
			return CommitEditStack{}, summaryErr
		}
		if len(summary.Parents) > 1 {
			return CommitEditStack{}, fmt.Errorf("merge commit %s is in the selected range; merge commits cannot be reordered", summary.ShortCommit)
		}
		commits = append(commits, CommitEditCommit{
			Author: summary.Author, Commit: summary.Commit, ShortCommit: summary.ShortCommit,
			Message: summary.Message, Date: summary.Date, Files: summary.Files, Parents: summary.Parents,
		})
	}
	defaultBranch := readDefaultBranch(ctx, repository, branch)
	return CommitEditStack{
		Branch: branch, DefaultBranch: defaultBranch, Head: head, Base: base,
		DefaultBranchTarget: branch == defaultBranch, Commits: commits,
	}, nil
}

func containsBranch(branches []string, target string) bool {
	for _, branch := range branches {
		if branch == target {
			return true
		}
	}
	return false
}

func (s *Service) CommitFileContent(ctx context.Context, oid, filePath string) (CommitFileContent, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return CommitFileContent{}, err
	}
	resolvedOID, err := resolveCommit(operationContext, repository, strings.TrimSpace(oid))
	if err != nil {
		return CommitFileContent{}, fmt.Errorf("resolve commit: %w", err)
	}
	path, err := validateCommitFilePath(strings.TrimSpace(filePath))
	if err != nil {
		return CommitFileContent{}, err
	}
	summary, err := readCommitSummary(operationContext, repository, resolvedOID)
	if err != nil {
		return CommitFileContent{}, err
	}
	if !commitChangesPath(summary.Files, path) {
		return CommitFileContent{}, fmt.Errorf("file %q is not changed by commit %s", path, summary.ShortCommit)
	}
	response := CommitFileContent{Commit: resolvedOID, Path: path, Editable: true}
	mode, exists, err := editableCommitFileMode(operationContext, repository, resolvedOID, summary.Parents, path)
	if err != nil {
		return CommitFileContent{}, err
	}
	if !isRegularGitFileMode(mode) {
		response.Editable = false
		response.Reason = nonRegularFileReason(mode)
		return response, nil
	}
	if !exists {
		if len(summary.Parents) == 0 {
			response.Reason = "The file is deleted by this commit and has no parent version to restore."
			return response, nil
		}
		restoredContent, restorable, restoreReason, readErr := readEditableCommitFileContent(operationContext, repository, summary.Parents[0], path)
		if readErr != nil {
			return CommitFileContent{}, readErr
		}
		response.Restorable = restorable
		response.RestoreContent = restoredContent
		if restorable {
			response.Reason = "The file is deleted by this commit. Restore file brings back its first-parent content."
		} else if restoreReason != "" {
			response.Editable = false
			response.Reason = restoreReason
		} else {
			response.Editable = false
			response.Reason = "The file is deleted by this commit and has no first-parent content to restore."
		}
		return response, nil
	}
	content, editable, reason, readErr := readEditableCommitFileContent(operationContext, repository, resolvedOID, path)
	if readErr != nil {
		return CommitFileContent{}, readErr
	}
	response.Exists = true
	response.Editable = editable
	response.Reason = reason
	response.Content = content
	return response, nil
}

func (s *Service) RewriteCommits(ctx context.Context, request RewriteCommitsRequest) (RewriteCommitsResponse, error) {
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return RewriteCommitsResponse{}, err
	}
	return s.rewriteCommits(operationContext, repository, request)
}

func (s *Service) rewriteCommits(ctx context.Context, repository *gitexec.Repository, request RewriteCommitsRequest) (RewriteCommitsResponse, error) {
	branch, err := currentLocalBranch(ctx, repository)
	if err != nil {
		return RewriteCommitsResponse{}, err
	}
	if branch != strings.TrimSpace(request.Branch) {
		return RewriteCommitsResponse{}, fmt.Errorf("current branch changed from %q to %q", request.Branch, branch)
	}
	head, err := resolveCommit(ctx, repository, "HEAD")
	if err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("resolve current HEAD: %w", err)
	}
	if head != strings.TrimSpace(request.ExpectedHead) {
		return RewriteCommitsResponse{}, errors.New("branch HEAD changed after the editor was opened; reload the edit stack")
	}
	base, err := resolveCommit(ctx, repository, strings.TrimSpace(request.Base))
	if err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("resolve rewrite base: %w", err)
	}
	defaultBranch := readDefaultBranch(ctx, repository, branch)
	if branch == defaultBranch && !request.ConfirmDefaultBranch {
		return RewriteCommitsResponse{}, fmt.Errorf("%s is the default branch; explicit confirmation is required before rewriting it", branch)
	}
	summaries, err := validateRewriteRange(ctx, repository, base, head, request.Commits)
	if err != nil {
		return RewriteCommitsResponse{}, err
	}
	provenanceNote, err := validateRewriteProvenanceNote(request.AppendRewriteProvenance, request.RewriteProvenanceNote)
	if err != nil {
		return RewriteCommitsResponse{}, err
	}
	committerName, committerEmail, err := readCommitterIdentity(ctx, repository)
	if err != nil {
		return RewriteCommitsResponse{}, err
	}
	fallbackState, err := s.snapshot(ctx, repository)
	if err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("snapshot repository before rewrite: %w", err)
	}

	temporaryRoot, err := os.MkdirTemp("", "gitgit-commit-rewrite-")
	if err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("create temporary rewrite directory: %w", err)
	}
	defer os.RemoveAll(temporaryRoot)
	if _, err := repository.Run(ctx, nil, "worktree", "add", "--quiet", "--detach", temporaryRoot, base); err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("create temporary rewrite worktree: %w", err)
	}
	worktreeAdded := true
	cleanupWorktree := func(cleanupContext context.Context) error {
		if !worktreeAdded {
			return nil
		}
		_, cleanupErr := repository.Run(cleanupContext, nil, "worktree", "remove", "--force", temporaryRoot)
		if cleanupErr == nil {
			worktreeAdded = false
		}
		return cleanupErr
	}
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = cleanupWorktree(cleanupContext)
	}()
	temporaryRepository := &gitexec.Repository{Root: temporaryRoot, CommonDir: repository.CommonDir, Runner: repository.Runner}
	for _, planned := range request.Commits {
		summary := summaries[planned.Commit]
		author, authorDate, err := rewriteCommitAuthorAndDate(summary, planned)
		if err != nil {
			return RewriteCommitsResponse{}, fmt.Errorf("validate replacement metadata for %s: %w", summary.ShortCommit, err)
		}
		if _, err := temporaryRepository.Run(ctx, nil, "cherry-pick", "--no-commit", planned.Commit); err != nil {
			return RewriteCommitsResponse{}, fmt.Errorf("apply %s in its new position: %w", summary.ShortCommit, err)
		}
		if err := applyCommitFileEdits(ctx, temporaryRepository, summary, planned.FileEdits); err != nil {
			return RewriteCommitsResponse{}, err
		}
		environment := []string{
			"GIT_AUTHOR_NAME=" + author.Name,
			"GIT_AUTHOR_EMAIL=" + author.Email,
			"GIT_AUTHOR_DATE=" + authorDate,
			"GIT_COMMITTER_NAME=" + committerName,
			"GIT_COMMITTER_EMAIL=" + committerEmail,
		}
		message := planned.Message
		if request.AppendRewriteProvenance {
			message = appendRewriteProvenance(message, summary.Commit, provenanceNote)
		}
		if _, err := temporaryRepository.Runner.RunWithEnv(ctx, temporaryRoot, strings.NewReader(message), environment,
			"commit", "--quiet", "--allow-empty", "--no-gpg-sign", "--file=-"); err != nil {
			return RewriteCommitsResponse{}, fmt.Errorf("create replacement for %s: %w", summary.ShortCommit, err)
		}
	}
	newHead, err := resolveCommit(ctx, temporaryRepository, "HEAD")
	if err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("resolve rewritten HEAD: %w", err)
	}
	if err := cleanupWorktree(ctx); err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("remove temporary rewrite worktree: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return RewriteCommitsResponse{}, err
	}

	criticalContext, cancelCritical := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancelCritical()
	if _, err := repository.Run(criticalContext, nil, "read-tree", "--dry-run", "-m", "-u", head, newHead); err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("rewrite cannot preserve the current worktree and index changes; commit or stash the overlapping changes, then retry: %w", err)
	}
	backupRef := fmt.Sprintf("refs/gitgit/backups/%s/%s", branch, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if _, err := repository.Run(criticalContext, nil, "update-ref", backupRef, head); err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("create recovery ref: %w", err)
	}
	branchRef := "refs/heads/" + branch
	if _, err := repository.Run(criticalContext, nil, "update-ref", branchRef, newHead, head); err != nil {
		return RewriteCommitsResponse{}, fmt.Errorf("move branch with lease: %w", err)
	}
	if _, err := repository.Run(criticalContext, nil, "read-tree", "-m", "-u", head, newHead); err != nil {
		rollbackErr := rollbackInstalledRewrite(repository, branchRef, head, newHead)
		if rollbackErr != nil {
			return RewriteCommitsResponse{}, fmt.Errorf("update worktree after rewrite: %w; rollback also failed: %v", err, rollbackErr)
		}
		return RewriteCommitsResponse{}, fmt.Errorf("update worktree after rewrite: %w", err)
	}
	installedHead, err := resolveCommit(criticalContext, repository, "HEAD")
	if err != nil || installedHead != newHead {
		return RewriteCommitsResponse{}, errors.New("branch moved concurrently while installing the rewritten commits; reload repository history")
	}
	refreshContext, cancelRefresh := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelRefresh()
	state, refreshErr := s.snapshot(refreshContext, repository)
	warning := ""
	if refreshErr != nil {
		state = fallbackState
		state.Head = newHead
		state.Dirty = true
		for index := range state.Worktrees {
			if canonicalWorktreePath(state.Worktrees[index].Path) == canonicalWorktreePath(repository.Root) {
				state.Worktrees[index].Head = newHead
				state.Worktrees[index].Dirty = true
			}
		}
		warning = fmt.Sprintf("rewrite installed, but repository refresh failed: %v", refreshErr)
	}
	return RewriteCommitsResponse{State: state, Head: newHead, BackupRef: backupRef, Warning: warning}, nil
}

func rollbackInstalledRewrite(repository *gitexec.Repository, branchRef, oldHead, newHead string) error {
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelRecovery()
	if _, err := repository.Run(recoveryContext, nil, "update-ref", branchRef, oldHead, newHead); err != nil {
		return err
	}
	if _, err := repository.Run(recoveryContext, nil, "read-tree", "-m", "-u", newHead, oldHead); err != nil {
		return fmt.Errorf("restore worktree and index after ref rollback: %w", err)
	}
	return nil
}

func validateRewriteRange(ctx context.Context, repository *gitexec.Repository, base, head string, planned []RewriteCommit) (map[string]CommitEditCommit, error) {
	if len(planned) == 0 {
		return nil, errors.New("at least one commit is required")
	}
	if len(planned) > maximumRewriteCommits {
		return nil, fmt.Errorf("GitGit edits at most %d commits at once", maximumRewriteCommits)
	}
	if _, err := repository.Run(ctx, nil, "merge-base", "--is-ancestor", base, head); err != nil {
		return nil, errors.New("the rewrite base is not an ancestor of the current branch HEAD")
	}
	out, err := repository.Run(ctx, nil, "rev-list", "--reverse", "--first-parent", base+".."+head)
	if err != nil {
		return nil, fmt.Errorf("read rewrite range: %w", err)
	}
	original := strings.Fields(string(out))
	if len(original) != len(planned) {
		return nil, errors.New("the edited commits no longer match the current branch range")
	}
	originalSet := make(map[string]bool, len(original))
	summaries := make(map[string]CommitEditCommit, len(original))
	for _, oid := range original {
		summary, err := readCommitSummary(ctx, repository, oid)
		if err != nil {
			return nil, err
		}
		if len(summary.Parents) > 1 {
			return nil, fmt.Errorf("merge commit %s is in the rewrite range", summary.ShortCommit)
		}
		originalSet[oid] = true
		summaries[oid] = CommitEditCommit{
			Author: summary.Author, Commit: summary.Commit, ShortCommit: summary.ShortCommit,
			Message: summary.Message, Date: summary.Date, Files: summary.Files, Parents: summary.Parents,
		}
	}
	seen := make(map[string]bool, len(planned))
	for _, commit := range planned {
		oid, err := resolveCommit(ctx, repository, strings.TrimSpace(commit.Commit))
		if err != nil || !originalSet[oid] || seen[oid] {
			return nil, errors.New("edited commits must be a permutation of the original rewrite range")
		}
		if strings.TrimSpace(commit.Message) == "" {
			return nil, fmt.Errorf("commit %s has an empty message", shortOID(oid))
		}
		if _, _, err := rewriteCommitAuthorAndDate(summaries[oid], commit); err != nil {
			return nil, fmt.Errorf("commit %s has invalid replacement metadata: %w", shortOID(oid), err)
		}
		seen[oid] = true
		summaries[commit.Commit] = summaries[oid]
	}
	return summaries, nil
}

func rewriteCommitAuthorAndDate(summary CommitEditCommit, planned RewriteCommit) (app.Author, string, error) {
	author := summary.Author
	if planned.Author != nil {
		name := strings.TrimSpace(planned.Author.Name)
		email := strings.TrimSpace(planned.Author.Email)
		if err := validateRewriteAuthor(name, email); err != nil {
			return app.Author{}, "", err
		}
		author = app.Author{Name: name, Email: email}
	}

	authorDate := summary.Date
	if planned.AuthorDate != nil {
		candidate := strings.TrimSpace(*planned.AuthorDate)
		if err := validateRewriteAuthorDate(candidate); err != nil {
			return app.Author{}, "", err
		}
		authorDate = candidate
	}
	return author, authorDate, nil
}

func validateRewriteAuthor(name, email string) error {
	if name == "" {
		return errors.New("author name is required")
	}
	if utf8.RuneCountInString(name) > maximumRewriteAuthorName {
		return fmt.Errorf("author name exceeds the %d character limit", maximumRewriteAuthorName)
	}
	if strings.ContainsAny(name, "\x00\r\n") {
		return errors.New("author name contains an invalid control character")
	}
	if email == "" {
		return errors.New("author email is required")
	}
	if utf8.RuneCountInString(email) > maximumRewriteAuthorEmail {
		return fmt.Errorf("author email exceeds the %d character limit", maximumRewriteAuthorEmail)
	}
	if strings.ContainsAny(email, "\x00\r\n") || strings.ContainsAny(email, "<>\t ") {
		return errors.New("author email must be a single email address")
	}
	if strings.Count(email, "@") != 1 || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return errors.New("author email must include a local part and domain")
	}
	return nil
}

func validateRewriteAuthorDate(value string) error {
	if value == "" {
		return errors.New("author date is required")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("author date contains an invalid control character")
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return errors.New("author date must use ISO 8601 with a timezone")
	}
	return nil
}

func validateRewriteProvenanceNote(enabled bool, note string) (string, error) {
	if !enabled {
		return "", nil
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maximumRewriteProvenanceNote {
		return "", fmt.Errorf("rewrite provenance note exceeds the %d character limit", maximumRewriteProvenanceNote)
	}
	if strings.ContainsAny(note, "\r\n") {
		return "", errors.New("rewrite provenance note must be a single line")
	}
	return note, nil
}

func appendRewriteProvenance(message, sourceOID, note string) string {
	message = stripTrailingRewriteProvenance(message)
	trailers := []string{rewriteProvenanceTrailer + " " + sourceOID}
	if note != "" {
		trailers = append(trailers, rewriteProvenanceNoteTrailer+" "+note)
	}
	return message + "\n\n" + strings.Join(trailers, "\n")
}

func stripTrailingRewriteProvenance(message string) string {
	lines := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	for end > 0 && isRewriteProvenanceTrailer(lines[end-1]) {
		end--
	}
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

func isRewriteProvenanceTrailer(line string) bool {
	return strings.HasPrefix(line, rewriteProvenanceTrailer+" ") ||
		strings.HasPrefix(line, legacyRewriteProvenanceTrailer+" ") ||
		strings.HasPrefix(line, rewriteProvenanceNoteTrailer+" ")
}

func applyCommitFileEdits(ctx context.Context, repository *gitexec.Repository, summary CommitEditCommit, edits []CommitFileEdit) error {
	seen := make(map[string]bool, len(edits))
	for _, edit := range edits {
		path, err := validateCommitFilePath(strings.TrimSpace(edit.Path))
		if err != nil {
			return err
		}
		if seen[path] {
			return fmt.Errorf("file %q is edited more than once in commit %s", path, summary.ShortCommit)
		}
		seen[path] = true
		if !commitChangesPath(summary.Files, path) {
			return fmt.Errorf("file %q is not changed by commit %s", path, summary.ShortCommit)
		}
		originalMode, _, err := editableCommitFileMode(ctx, repository, summary.Commit, summary.Parents, path)
		if err != nil {
			return fmt.Errorf("inspect original file mode for %q: %w", path, err)
		}
		if !isRegularGitFileMode(originalMode) {
			return fmt.Errorf("file %q cannot be edited: %s", path, nonRegularFileReason(originalMode))
		}
		indexMode, indexExists, err := indexFileMode(ctx, repository, path)
		if err != nil {
			return fmt.Errorf("inspect staged file mode for %q: %w", path, err)
		}
		if indexExists && !isRegularGitFileMode(indexMode) {
			return fmt.Errorf("file %q became a non-regular file while rewriting commits", path)
		}
		absolutePath, err := prepareRewriteFilePath(repository.Root, path, !edit.Delete)
		if err != nil {
			return err
		}
		if edit.Delete {
			if err := os.Remove(absolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete %q: %w", path, err)
			}
		} else {
			if len(edit.Content) > maximumEditableFile {
				return fmt.Errorf("file %q exceeds the 2 MiB edit limit", path)
			}
			if strings.IndexByte(edit.Content, 0) >= 0 {
				return fmt.Errorf("file %q contains NUL bytes and cannot be edited as text", path)
			}
			writeMode := originalMode
			if indexExists {
				writeMode = indexMode
			}
			if err := writeRewriteFile(repository.Root, path, edit.Content, gitFileMode(writeMode)); err != nil {
				return fmt.Errorf("write %q: %w", path, err)
			}
		}
		if _, err := repository.Run(ctx, nil, "add", "--all", "--", ":(literal)"+path); err != nil {
			return fmt.Errorf("stage edited file %q: %w", path, err)
		}
	}
	return nil
}

func editableCommitFileMode(ctx context.Context, repository *gitexec.Repository, commit string, parents []string, path string) (string, bool, error) {
	mode, exists, err := treeFileMode(ctx, repository, commit, path)
	if err != nil || exists {
		return mode, exists, err
	}
	if len(parents) == 0 {
		return "", false, nil
	}
	mode, parentExists, err := treeFileMode(ctx, repository, parents[0], path)
	if err != nil {
		return "", false, err
	}
	if !parentExists {
		return "", false, nil
	}
	return mode, false, nil
}

func readEditableCommitFileContent(ctx context.Context, repository *gitexec.Repository, commit, path string) (string, bool, string, error) {
	mode, exists, err := treeFileMode(ctx, repository, commit, path)
	if err != nil {
		return "", false, "", err
	}
	if !exists {
		return "", false, "", nil
	}
	if !isRegularGitFileMode(mode) {
		return "", false, nonRegularFileReason(mode), nil
	}
	object := commit + ":" + path
	sizeOutput, err := repository.Run(ctx, nil, "cat-file", "-s", object)
	if err != nil {
		return "", false, "", fmt.Errorf("read file size: %w", err)
	}
	var size int64
	if _, err := fmt.Sscan(strings.TrimSpace(string(sizeOutput)), &size); err != nil {
		return "", false, "", fmt.Errorf("parse file size: %w", err)
	}
	if size > maximumEditableFile {
		return "", false, "Files larger than 2 MiB are not editable in GitGit.", nil
	}
	content, err := repository.Run(ctx, nil, "show", object)
	if err != nil {
		return "", false, "", fmt.Errorf("read file content: %w", err)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return "", false, "Binary files are not editable in GitGit.", nil
	}
	return string(content), true, "", nil
}

func treeFileMode(ctx context.Context, repository *gitexec.Repository, treeish, path string) (string, bool, error) {
	out, err := repository.Run(ctx, nil, "ls-tree", "-z", "--end-of-options", treeish, "--", ":(literal)"+path)
	if err != nil {
		return "", false, fmt.Errorf("read tree entry: %w", err)
	}
	records := bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	if len(records) == 1 && len(records[0]) == 0 {
		return "", false, nil
	}
	if len(records) != 1 {
		return "", false, fmt.Errorf("Git returned %d tree entries for %q", len(records), path)
	}
	metadata, returnedPath, found := bytes.Cut(records[0], []byte{'\t'})
	if !found || string(returnedPath) != path {
		return "", false, fmt.Errorf("Git returned an unexpected tree entry for %q", path)
	}
	fields := strings.Fields(string(metadata))
	if len(fields) != 3 {
		return "", false, fmt.Errorf("Git returned invalid tree metadata for %q", path)
	}
	return fields[0], true, nil
}

func indexFileMode(ctx context.Context, repository *gitexec.Repository, path string) (string, bool, error) {
	out, err := repository.Run(ctx, nil, "ls-files", "--stage", "-z", "--", ":(literal)"+path)
	if err != nil {
		return "", false, err
	}
	records := bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	if len(records) == 1 && len(records[0]) == 0 {
		return "", false, nil
	}
	if len(records) != 1 {
		return "", false, fmt.Errorf("Git returned %d index entries for %q", len(records), path)
	}
	metadata, returnedPath, found := bytes.Cut(records[0], []byte{'\t'})
	if !found || string(returnedPath) != path {
		return "", false, fmt.Errorf("Git returned an unexpected index entry for %q", path)
	}
	fields := strings.Fields(string(metadata))
	if len(fields) != 3 || fields[2] != "0" {
		return "", false, fmt.Errorf("Git returned an unmerged or invalid index entry for %q", path)
	}
	return fields[0], true, nil
}

func isRegularGitFileMode(mode string) bool {
	return mode == "100644" || mode == "100755"
}

func nonRegularFileReason(mode string) string {
	if mode == "120000" {
		return "Symbolic links are not editable in GitGit."
	}
	return "Only regular file blobs can be edited."
}

func gitFileMode(mode string) os.FileMode {
	if mode == "100755" {
		return 0o755
	}
	return 0o644
}

func prepareRewriteFilePath(root, path string, createParents bool) (string, error) {
	parts := strings.Split(filepath.FromSlash(path), string(filepath.Separator))
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && createParents {
			if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("create parent directory for %q: %w", path, err)
			}
			info, err = os.Lstat(current)
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect parent directory for %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("file %q has a symlink or non-directory parent", path)
		}
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Lstat(absolutePath)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return "", fmt.Errorf("file %q is a symlink or non-regular file", path)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect file %q: %w", path, err)
	}
	return absolutePath, nil
}

func writeRewriteFile(root, path, content string, mode os.FileMode) error {
	absolutePath, err := prepareRewriteFilePath(root, path, true)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(absolutePath), ".gitgit-edit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, err := prepareRewriteFilePath(root, path, false); err != nil {
		return err
	}
	return os.Rename(temporaryPath, absolutePath)
}

func currentLocalBranch(ctx context.Context, repository *gitexec.Repository) (string, error) {
	out, err := repository.Run(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", errors.New("commit editing requires a checked-out local branch; detached HEAD is not supported")
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", errors.New("Git returned an empty current branch")
	}
	return branch, nil
}

func resolveCommit(ctx context.Context, repository *gitexec.Repository, revision string) (string, error) {
	if err := validateRevisionExpression("commit revision", revision); err != nil {
		return "", err
	}
	out, err := repository.Run(ctx, nil, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func readCommitterIdentity(ctx context.Context, repository *gitexec.Repository) (string, string, error) {
	nameOutput, nameErr := repository.Run(ctx, nil, "config", "--get", "user.name")
	emailOutput, emailErr := repository.Run(ctx, nil, "config", "--get", "user.email")
	name := strings.TrimSpace(string(nameOutput))
	email := strings.TrimSpace(string(emailOutput))
	if nameErr != nil || emailErr != nil || name == "" || email == "" {
		return "", "", errors.New("configure Git user.name and user.email before editing commits")
	}
	return name, email, nil
}

func validateCommitFilePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", errors.New("file path must be repository-relative")
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.ContainsRune(cleaned, 0) {
		return "", errors.New("file path escapes the repository")
	}
	return cleaned, nil
}

func commitChangesPath(files []app.FileChange, path string) bool {
	for _, file := range files {
		if file.Path == path || file.OldPath == path {
			return true
		}
	}
	return false
}
