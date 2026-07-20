package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CommandError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	message := strings.TrimSpace(e.Stderr)
	if message == "" {
		message = e.Err.Error()
	}
	return message
}

func (e *CommandError) Unwrap() error { return e.Err }

type Runner struct {
	Binary string
}

func NewRunner() *Runner { return &Runner{Binary: "git"} }

func (r *Runner) Run(ctx context.Context, dir string, stdin io.Reader, args ...string) ([]byte, error) {
	return r.RunWithEnv(ctx, dir, stdin, nil, args...)
}

// RunWithEnv runs Git with task-specific environment overrides. Values in
// environment take precedence over the process environment.
func (r *Runner) RunWithEnv(ctx context.Context, dir string, stdin io.Reader, environment []string, args ...string) ([]byte, error) {
	if r.Binary == "" {
		r.Binary = "git"
	}
	commandArgs := make([]string, 0, len(args)+3)
	if dir != "" {
		commandArgs = append(commandArgs, "-C", dir)
	}
	commandArgs = append(commandArgs, "--no-pager")
	commandArgs = append(commandArgs, args...)

	cmd := exec.CommandContext(ctx, r.Binary, commandArgs...)
	cmd.Stdin = stdin
	cmd.Env = append(sanitizedGitEnvironment(), "LC_ALL=C", "GIT_PAGER=cat")
	cmd.Env = append(cmd.Env, environment...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &CommandError{Args: commandArgs, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}

func sanitizedGitEnvironment() []string {
	environment := os.Environ()
	sanitized := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if isRepositorySelectionEnvironment(key) || key == "LC_ALL" || key == "GIT_PAGER" {
			continue
		}
		sanitized = append(sanitized, entry)
	}
	return sanitized
}

func isRepositorySelectionEnvironment(key string) bool {
	switch key {
	case "GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_COMMON_DIR",
		"GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_NAMESPACE",
		"GIT_SHALLOW_FILE",
		"GIT_REPLACE_REF_BASE",
		"GIT_GRAFT_FILE",
		"GIT_CEILING_DIRECTORIES",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM",
		"GIT_CONFIG",
		"GIT_CONFIG_GLOBAL",
		"GIT_CONFIG_SYSTEM",
		"GIT_CONFIG_PARAMETERS",
		"GIT_CONFIG_COUNT":
		return true
	default:
		return strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_")
	}
}

func (r *Runner) Version(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "", nil, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type Repository struct {
	Root      string
	CommonDir string
	Runner    *Runner
}

func OpenRepository(ctx context.Context, runner *Runner, path string) (*Repository, error) {
	if runner == nil {
		runner = NewRunner()
	}
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	out, err := runner.Run(ctx, abs, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a Git worktree: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return nil, errors.New("Git returned an empty worktree root")
	}
	commonDirOut, err := runner.Run(ctx, root, nil, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Git common directory: %w", err)
	}
	commonDir := strings.TrimSpace(string(commonDirOut))
	if commonDir == "" {
		return nil, errors.New("Git returned an empty common directory")
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir = filepath.Clean(commonDir)
	if resolved, resolveErr := filepath.EvalSymlinks(commonDir); resolveErr == nil {
		commonDir = filepath.Clean(resolved)
	}
	return &Repository{Root: root, CommonDir: commonDir, Runner: runner}, nil
}

func (r *Repository) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	return r.Runner.Run(ctx, r.Root, stdin, args...)
}
