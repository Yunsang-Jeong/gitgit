package app

import (
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

const defaultRandomTestSeed int64 = 20260718

func deterministicRand(t *testing.T, salt int64) *rand.Rand {
	t.Helper()
	seed := defaultRandomTestSeed
	if raw := os.Getenv("RANDOM_TEST_SEED"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			t.Fatalf("RANDOM_TEST_SEED=%q is not an integer: %v", raw, err)
		}
		seed = parsed
	}
	seed ^= salt * 0x5DEECE66D
	t.Logf("deterministic random seed: %d", seed)
	return rand.New(rand.NewSource(seed))
}

func randomWord(rng *rand.Rand, prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	length := 7 + rng.Intn(8)
	var value strings.Builder
	value.Grow(len(prefix) + length + 1)
	value.WriteString(prefix)
	value.WriteByte('_')
	for range length {
		value.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return value.String()
}

func gitCommitAs(t *testing.T, root, authorName, authorEmail, date, message string) string {
	t.Helper()
	gitTestCommand(t, root, "add", "--all")
	command := exec.Command("git", "-C", root, "commit", "--quiet", "-m", message)
	command.Env = append(isolatedGitEnvironment(),
		"GIT_AUTHOR_NAME="+authorName,
		"GIT_AUTHOR_EMAIL="+authorEmail,
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=GitGit Test Bot",
		"GIT_COMMITTER_EMAIL=tests@gitgit.local",
		"GIT_COMMITTER_DATE="+date,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("commit %q: %v\n%s", message, err, output)
	}
	return strings.TrimSpace(gitTestCommand(t, root, "rev-parse", "HEAD"))
}

func isolatedGitEnvironment() []string {
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"LC_ALL=C",
		"TZ=UTC",
	)
}
