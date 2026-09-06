package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// The budgets are wall-clock ceilings, not targets. They sit at roughly twice
// the slowest run observed on the reference machine (Apple silicon, warm page
// cache) so an algorithmic regression - an extra process per commit, a lost
// batch, a dropped cache - fails the gate while ordinary hardware and load
// variation does not. Raise one only with a measurement that justifies it.
const (
	defaultOpenBudget           = 2000 * time.Millisecond
	defaultFirstHistoryBudget   = 6000 * time.Millisecond
	defaultFurtherHistoryBudget = 3000 * time.Millisecond
	largeHistoryPageSize        = 100
	largeMeasurementRounds      = 3
)

// This gate is opt-in rather than part of the default test run. It is timing
// dependent, so a busy machine running `task check` would fail here for
// reasons that have nothing to do with the change under test. `task
// check:performance` sets the fixture path; without it the gate skips, as it
// also does when the clone is absent, because a large fixture cannot be built
// on demand the way subgit is.
func largeFixturePath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("GITGIT_LARGE_FIXTURE")
	if path == "" {
		t.Skip("set GITGIT_LARGE_FIXTURE or run `task check:performance` to measure the load budget")
	}
	if info, err := os.Stat(filepath.Join(path, ".git")); err != nil || !info.IsDir() {
		t.Skipf("large fixture is missing at %s; run `task fixture:large` to create it", path)
	}
	return path
}

func budgetFromEnv(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	milliseconds, err := strconv.Atoi(raw)
	if err != nil || milliseconds <= 0 {
		t.Fatalf("%s must be a positive number of milliseconds, got %q", name, raw)
	}
	return time.Duration(milliseconds) * time.Millisecond
}

// Repeat each measurement and keep the fastest. Noise only ever adds time, so
// the minimum is the stable answer to "can the code do this at all", which is
// what a ceiling should be judged against.
func TestLargeRepositoryLoadBudget(t *testing.T) {
	fixture := largeFixturePath(t)
	openBudget := budgetFromEnv(t, "GITGIT_OPEN_BUDGET_MS", defaultOpenBudget)
	firstBudget := budgetFromEnv(t, "GITGIT_FIRST_HISTORY_BUDGET_MS", defaultFirstHistoryBudget)
	furtherBudget := budgetFromEnv(t, "GITGIT_FURTHER_HISTORY_BUDGET_MS", defaultFurtherHistoryBudget)

	var (
		bestOpen    = time.Duration(1<<63 - 1)
		bestFirst   = time.Duration(1<<63 - 1)
		bestFurther = time.Duration(1<<63 - 1)
		commitCount int
		branch      string
	)

	for round := 0; round < largeMeasurementRounds; round++ {
		service := NewService(nil)

		started := time.Now()
		state, err := service.Open(context.Background(), fixture)
		elapsed := time.Since(started)
		if err != nil {
			_ = service.Close()
			t.Fatalf("open %s: %v", fixture, err)
		}
		bestOpen = min(bestOpen, elapsed)
		branch = state.Branch

		started = time.Now()
		first, err := service.History(context.Background(), HistoryRequest{Scope: state.Branch, Limit: largeHistoryPageSize})
		elapsed = time.Since(started)
		if err != nil {
			_ = service.Close()
			t.Fatalf("read the first history page: %v", err)
		}
		bestFirst = min(bestFirst, elapsed)
		commitCount = len(first.Commits)

		started = time.Now()
		if _, err := service.History(context.Background(), HistoryRequest{
			Scope: state.Branch,
			Limit: largeHistoryPageSize,
			Skip:  largeHistoryPageSize,
		}); err != nil {
			_ = service.Close()
			t.Fatalf("read the second history page: %v", err)
		}
		bestFurther = min(bestFurther, time.Since(started))

		_ = service.Close()
	}

	if commitCount != largeHistoryPageSize {
		t.Fatalf("first page returned %d commits, want %d; the fixture is too small to be a load gate", commitCount, largeHistoryPageSize)
	}

	t.Logf("fixture=%s branch=%s open=%s first_history=%s further_history=%s",
		fixture, branch, roundMS(bestOpen), roundMS(bestFirst), roundMS(bestFurther))

	for _, measurement := range []struct {
		name   string
		best   time.Duration
		budget time.Duration
		env    string
	}{
		{"repository open", bestOpen, openBudget, "GITGIT_OPEN_BUDGET_MS"},
		{"first history page", bestFirst, firstBudget, "GITGIT_FIRST_HISTORY_BUDGET_MS"},
		{"further history page", bestFurther, furtherBudget, "GITGIT_FURTHER_HISTORY_BUDGET_MS"},
	} {
		if measurement.best > measurement.budget {
			t.Errorf("%s took %s, over the %s budget; investigate the regression or raise %s with a measurement",
				measurement.name, roundMS(measurement.best), roundMS(measurement.budget), measurement.env)
		}
	}
}

func roundMS(value time.Duration) string {
	return fmt.Sprintf("%dms", value.Round(time.Millisecond).Milliseconds())
}
