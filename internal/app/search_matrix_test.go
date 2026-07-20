package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestSearchRandomizedHistoryMatrix(t *testing.T) {
	t.Parallel()
	rng := deterministicRand(t, 101)
	root := newTestRepository(t)
	baseToken := randomWord(rng, "base")
	messageToken := randomWord(rng, "message")
	diffToken := randomWord(rng, "diff")
	countToken := randomWord(rng, "count")
	filenameToken := randomWord(rng, "filename")
	bulkToken := randomWord(rng, "bulk")
	sideToken := randomWord(rng, "side")

	writeTestFile(t, root, "base/root.txt", "base="+baseToken+"\n")
	gitCommitAs(t, root, "Alice Example", "alice@example.com", "2024-01-10T09:00:00+09:00", "chore: seed "+baseToken)
	gitTestCommand(t, root, "tag", "matrix-base")

	targetPath := filepath.ToSlash(filepath.Join("src", filenameToken+".txt"))
	writeTestFile(t, root, targetPath, "header=stable\nsignal="+diffToken+"\nfooter=stable\n")
	targetOID := gitCommitAs(t, root, "Bob Example", "bob@example.com", "2024-03-15T10:30:00+09:00", "feat: "+messageToken)

	writeTestFile(t, root, targetPath, "header=stable\nsignal="+diffToken+"\nfooter=stable\ncount="+countToken+"\n")
	gitCommitAs(t, root, "Alice Example", "alice@example.com", "2024-06-20T14:00:00+09:00", "fix: "+randomWord(rng, "followup"))

	docsPath := filepath.ToSlash(filepath.Join("docs", randomWord(rng, "guide")+" notes.md"))
	writeTestFile(t, root, docsPath, "signal="+diffToken+"\n")
	gitCommitAs(t, root, "Carol Example", "carol@example.com", "2025-02-11T08:15:00+09:00", "docs: "+randomWord(rng, "history"))

	bulkPaths := make([]string, 0, 5)
	for index := range 5 {
		path := filepath.ToSlash(filepath.Join("bulk", fmt.Sprintf("%02d-%s.txt", index, randomWord(rng, "entry"))))
		bulkPaths = append(bulkPaths, path)
		writeTestFile(t, root, path, "marker="+bulkToken+"\n")
	}
	gitCommitAs(t, root, "Dana Example", "dana@example.com", "2025-08-21T11:20:00+09:00", "test: bulk "+bulkToken)

	gitTestCommand(t, root, "switch", "-c", "matrix-side")
	sidePath := filepath.ToSlash(filepath.Join("side", randomWord(rng, "branch")+".txt"))
	writeTestFile(t, root, sidePath, "side="+sideToken+"\n")
	gitCommitAs(t, root, "Eve Example", "eve@example.com", "2026-01-12T13:10:00+09:00", "feat: "+sideToken)
	gitTestCommand(t, root, "switch", "main")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)

	tests := []struct {
		name        string
		options     SearchOptions
		wantCount   int
		wantPaths   []string
		wantSources []string
	}{
		{
			name:      "message glob",
			options:   searchOptions("*"+messageToken+"*", "message"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"msg"},
		},
		{
			name:      "message regex",
			options:   withSearchRegex(searchOptions("^feat: "+regexp.QuoteMeta(messageToken)+"$", "message")),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"msg"},
		},
		{
			name:      "changed line glob",
			options:   searchOptions("*"+diffToken+"*", "diff"),
			wantCount: 2, wantPaths: []string{docsPath, targetPath}, wantSources: []string{"diff"},
		},
		{
			name:      "changed line regex",
			options:   withSearchRegex(searchOptions("signal="+regexp.QuoteMeta(diffToken), "diff")),
			wantCount: 2, wantPaths: []string{docsPath, targetPath}, wantSources: []string{"diff"},
		},
		{
			name:      "filename across multiple changes",
			options:   searchOptions("**/*"+filenameToken+"*", "file"),
			wantCount: 2, wantPaths: []string{targetPath, targetPath}, wantSources: []string{"file"},
		},
		{
			name:      "author intersects diff matches",
			options:   withAuthor(searchOptions("*"+diffToken+"*", "diff"), "Bob.*"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"diff"},
		},
		{
			name:      "date window",
			options:   withDates(searchOptions("*"+messageToken+"*", "message"), "2024-03-01T00:00:00+09:00", "2024-03-31T23:59:59+09:00"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"msg"},
		},
		{
			name:      "path filter",
			options:   withPaths(searchOptions("*"+diffToken+"*", "diff"), "src"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"diff"},
		},
		{
			name:      "revision range",
			options:   withRevision(searchOptions("*"+messageToken+"*", "message"), "matrix-base..main"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"msg"},
		},
		{
			name:      "one exact commit",
			options:   withRevision(searchOptions("*"+diffToken+"*", "diff"), targetOID+"^!"),
			wantCount: 1, wantPaths: []string{targetPath}, wantSources: []string{"diff"},
		},
		{
			name:      "symmetric difference",
			options:   withRevision(searchOptions("*"+sideToken+"*", "message"), "main...matrix-side"),
			wantCount: 1, wantPaths: []string{sidePath}, wantSources: []string{"msg"},
		},
		{
			name:      "side branch excluded by default",
			options:   searchOptions("*"+sideToken+"*", "all"),
			wantCount: 0,
		},
		{
			name:      "all refs includes side branch",
			options:   withAll(searchOptions("*"+sideToken+"*", "all")),
			wantCount: 1, wantPaths: []string{sidePath}, wantSources: []string{"msg", "diff"},
		},
		{
			name:      "limit stops within multi-file commit",
			options:   withLimit(searchOptions("*"+bulkToken+"*", "all"), 3),
			wantCount: 3, wantPaths: bulkPaths[:3], wantSources: []string{"msg", "diff"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := service.Search(context.Background(), test.options)
			if err != nil {
				t.Fatal(err)
			}
			if response.Count != test.wantCount || len(response.Results) != test.wantCount {
				t.Fatalf("count = %d/%d, want %d: %#v", response.Count, len(response.Results), test.wantCount, response.Results)
			}
			gotPaths := make([]string, 0, len(response.Results))
			for _, result := range response.Results {
				gotPaths = append(gotPaths, result.File.Path)
				if test.wantSources != nil && !slices.Equal(result.MatchSources, test.wantSources) {
					t.Errorf("sources for %s = %v, want %v", result.File.Path, result.MatchSources, test.wantSources)
				}
			}
			slices.Sort(gotPaths)
			wantPaths := slices.Clone(test.wantPaths)
			slices.Sort(wantPaths)
			if !slices.Equal(gotPaths, wantPaths) {
				t.Fatalf("paths = %v, want %v", gotPaths, wantPaths)
			}
		})
	}

	contextZero := searchOptions("*"+countToken+"*", "diff")
	contextZero.Context = 0
	response, err := service.Search(context.Background(), contextZero)
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || strings.Contains(response.Results[0].Diff, "header=stable") {
		t.Fatalf("zero-context diff included surrounding lines: %#v", response.Results)
	}
}

func TestSearchCaseOnlyRenameAndCurrentPathFollow(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	oldPath := "a/b/c/d.txt"
	newPath := "a/b/c/D.txt"
	writeTestFile(t, root, oldPath, "before\n")
	gitCommitAs(t, root, "Alice Example", "alice@example.com", "2024-01-01T00:00:00Z", "add lowercase path")
	gitTestCommand(t, root, "mv", "-f", oldPath, newPath)
	gitCommitAs(t, root, "Bob Example", "bob@example.com", "2024-02-01T00:00:00Z", "rename path case")
	writeTestFile(t, root, newPath, "before\nafter-case-rename\n")
	gitCommitAs(t, root, "Carol Example", "carol@example.com", "2024-03-01T00:00:00Z", "modify uppercase path")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)
	lowercase, err := service.Search(context.Background(), searchOptions(oldPath, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if lowercase.Count != 1 {
		t.Fatalf("lowercase filename count = %d, want 1 without rename following: %#v", lowercase.Count, lowercase.Results)
	}
	if lowercase.Results[0].File.Path != oldPath {
		t.Fatalf("unexpected lowercase file result: %#v", lowercase.Results[0].File)
	}

	followOptions := searchOptions(newPath, "file")
	followOptions.FollowRename = true
	follow, err := service.Search(context.Background(), followOptions)
	if err != nil {
		t.Fatal(err)
	}
	if follow.Count != 3 {
		t.Fatalf("follow count = %d, want 3: %#v", follow.Count, follow.Results)
	}
	if !strings.Contains(follow.Results[0].Diff, "after-case-rename") {
		t.Fatalf("follow missed the post-rename commit: %#v", follow.Results)
	}

	allRefsOptions := followOptions
	allRefsOptions.All = true
	allRefs, err := service.Search(context.Background(), allRefsOptions)
	if err != nil {
		t.Fatal(err)
	}
	if allRefs.Count != 3 {
		t.Fatalf("all-refs follow count = %d, want 3: %#v", allRefs.Count, allRefs.Results)
	}
}

func TestSearchValidationMatrix(t *testing.T) {
	valid := searchOptions("*query*", "all")
	tests := []struct {
		name string
		edit func(*SearchOptions)
		code string
	}{
		{name: "missing patterns", edit: func(o *SearchOptions) { o.Messages = nil; o.Diffs = nil; o.Files = nil }, code: "missing_search_pattern"},
		{name: "follow without file", edit: func(o *SearchOptions) { o.Files = nil; o.FollowRename = true }, code: "invalid_arguments"},
		{name: "follow and path", edit: func(o *SearchOptions) { o.FollowRename = true; o.Paths = []string{"src"} }, code: "invalid_arguments"},
		{name: "all and revision", edit: func(o *SearchOptions) { o.All = true; o.Revision = "main" }, code: "invalid_arguments"},
		{name: "engine", edit: func(o *SearchOptions) { o.Engine = "unknown" }, code: "invalid_engine"},
		{name: "zero limit", edit: func(o *SearchOptions) { o.Limit = 0 }, code: "invalid_limit"},
		{name: "negative context", edit: func(o *SearchOptions) { o.Context = -1 }, code: "invalid_context"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.edit(&options)
			err := validateSearchOptions(options)
			if err == nil {
				t.Fatal("validation unexpectedly succeeded")
			}
			code, _, _ := apperr.Details(err)
			if code != test.code {
				t.Fatalf("error code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestSearchSourceEngineScopeCombinationMatrix(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "src/matrix-target.txt", "signal=MATRIX_DIFF\n")
	gitCommitAs(t, root, "Alice Example", "alice@example.com", "2025-01-01T00:00:00Z", "feat: MATRIX_MESSAGE main")
	gitTestCommand(t, root, "switch", "-c", "matrix-side")
	writeTestFile(t, root, "src/matrix-side.txt", "signal=MATRIX_DIFF\n")
	gitCommitAs(t, root, "Bob Example", "bob@example.com", "2025-02-01T00:00:00Z", "feat: MATRIX_MESSAGE side")
	gitTestCommand(t, root, "switch", "main")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)
	type sourcePattern struct {
		source string
		glob   string
		regex  string
	}
	sources := []sourcePattern{
		{source: "msg", glob: "*MATRIX_MESSAGE*", regex: "MATRIX_MESSAGE"},
		{source: "diff", glob: "*MATRIX_DIFF*", regex: "MATRIX_DIFF"},
		{source: "file", glob: "src/matrix-*.txt", regex: `^src/matrix-(target|side)\.txt$`},
	}

	for _, engine := range []string{"glob", "regex"} {
		for _, allRefs := range []bool{false, true} {
			for mask := 1; mask < 1<<len(sources); mask++ {
				options := SearchOptions{Engine: engine, All: allRefs, Limit: 100, Context: 3}
				wantSources := make([]string, 0, len(sources))
				for index, source := range sources {
					if mask&(1<<index) == 0 {
						continue
					}
					pattern := source.glob
					if engine == "regex" {
						pattern = source.regex
					}
					switch source.source {
					case "msg":
						options.Messages = []string{pattern}
					case "diff":
						options.Diffs = []string{pattern}
					case "file":
						options.Files = []string{pattern}
						options.FollowRename = true
					}
					wantSources = append(wantSources, source.source)
				}
				name := fmt.Sprintf("engine=%s/all=%t/sources=%s", engine, allRefs, strings.Join(wantSources, "+"))
				t.Run(name, func(t *testing.T) {
					response, err := service.Search(context.Background(), options)
					if err != nil {
						t.Fatal(err)
					}
					wantCount := 1
					if allRefs {
						wantCount = 2
					}
					if response.Count != wantCount {
						t.Fatalf("count = %d, want %d: %#v", response.Count, wantCount, response.Results)
					}
					for _, result := range response.Results {
						if !slices.Equal(result.MatchSources, wantSources) {
							t.Fatalf("sources = %v, want %v", result.MatchSources, wantSources)
						}
					}
				})
			}
		}
	}
}

func TestSearchAuthorSinceUntilCombinationMatrix(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	for _, fixture := range []struct {
		path, author, email, date string
	}{
		{path: "old.txt", author: "Alice Example", email: "alice@example.com", date: "2024-01-01T00:00:00Z"},
		{path: "target.txt", author: "Bob Example", email: "bob@example.com", date: "2025-06-15T00:00:00Z"},
		{path: "recent.txt", author: "Carol Example", email: "carol@example.com", date: "2026-01-01T00:00:00Z"},
	} {
		writeTestFile(t, root, fixture.path, "MATRIX_FILTER\n")
		gitCommitAs(t, root, fixture.author, fixture.email, fixture.date, "feat: MATRIX_FILTER")
	}

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)
	wantCounts := map[int]int{0: 3, 1: 1, 2: 2, 3: 1, 4: 2, 5: 1, 6: 1, 7: 1}
	for mask := 0; mask < 8; mask++ {
		options := searchOptions("*MATRIX_FILTER*", "message")
		if mask&1 != 0 {
			options.Author = "Bob Example"
		}
		if mask&2 != 0 {
			options.Since = "2025-01-01"
		}
		if mask&4 != 0 {
			options.Until = "2025-12-31"
		}
		t.Run(fmt.Sprintf("author=%t/since=%t/until=%t", mask&1 != 0, mask&2 != 0, mask&4 != 0), func(t *testing.T) {
			response, err := service.Search(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if response.Count != wantCounts[mask] {
				t.Fatalf("count = %d, want %d: %#v", response.Count, wantCounts[mask], response.Results)
			}
		})
	}
}

func TestSearchSinceFiltersNonMonotonicCommitDates(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "parent.txt", "NEWER_PARENT_TOKEN\n")
	gitCommitAs(t, root, "Alice Example", "alice@example.com", "2026-01-01T00:00:00Z", "newer parent token")
	writeTestFile(t, root, "child.txt", "older child\n")
	gitCommitAs(t, root, "Bob Example", "bob@example.com", "2020-01-01T00:00:00Z", "older child date")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	options := searchOptions("*NEWER_PARENT_TOKEN*", "diff")
	options.Since = "2025-01-01T00:00:00Z"
	response, err := NewSearchService(repo).Search(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || response.Results[0].File.Path != "parent.txt" {
		t.Fatalf("since filter missed newer-dated parent: %#v", response.Results)
	}
}

func TestChangedLinesDistinguishesHeadersFromContent(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/file.txt b/file.txt",
		"index 1111111..2222222 100644",
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,2 +1,2 @@",
		"---REMOVED_CONTENT",
		"+++ADDED_CONTENT",
		" context",
	}, "\n")
	got := changedLines(diff)
	if !strings.Contains(got, "--REMOVED_CONTENT") || !strings.Contains(got, "++ADDED_CONTENT") {
		t.Fatalf("changedLines dropped content resembling headers: %q", got)
	}
	if strings.Contains(got, "a/file.txt") || strings.Contains(got, "b/file.txt") {
		t.Fatalf("changedLines included diff headers: %q", got)
	}
}

func FuzzNewMatcher(f *testing.F) {
	for _, seed := range []string{"plain", "a+b[0]", "한글", "", "UPPER_lower"} {
		f.Add(seed, "prefix "+seed+" suffix")
	}
	f.Fuzz(func(t *testing.T, pattern, value string) {
		if glob, err := newMatcher(pattern, "glob"); err == nil {
			_ = glob(value)
		}
		if regex, err := newMatcher(pattern, "regex"); err == nil {
			_ = regex(value)
		}
	})
}

func searchOptions(pattern, source string) SearchOptions {
	options := SearchOptions{Engine: "glob", Limit: 100, Context: 3}
	switch source {
	case "message":
		options.Messages = []string{pattern}
	case "diff":
		options.Diffs = []string{pattern}
	case "file":
		options.Files = []string{pattern}
	case "all":
		options.Messages = []string{pattern}
		options.Diffs = []string{pattern}
		options.Files = []string{pattern}
	default:
		panic("unsupported test search source: " + source)
	}
	return options
}

func withSearchRegex(options SearchOptions) SearchOptions {
	options.Engine = "regex"
	return options
}

func withAuthor(options SearchOptions, author string) SearchOptions {
	options.Author = author
	return options
}

func withDates(options SearchOptions, since, until string) SearchOptions {
	options.Since = since
	options.Until = until
	return options
}

func withPaths(options SearchOptions, paths ...string) SearchOptions {
	options.Paths = paths
	return options
}

func withRevision(options SearchOptions, revision string) SearchOptions {
	options.Revision = revision
	return options
}

func withAll(options SearchOptions) SearchOptions {
	options.All = true
	return options
}

func withLimit(options SearchOptions, limit int) SearchOptions {
	options.Limit = limit
	return options
}

func TestSearchRejectsInvalidRegexBeforeGit(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "file.txt", "content\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2024-01-01T00:00:00Z", "initial")
	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	options := searchOptions("[", "diff")
	options.Engine = "regex"
	_, err = NewSearchService(repo).Search(context.Background(), options)
	if err == nil {
		t.Fatal("non-POSIX pattern unexpectedly succeeded")
	}
	code, _, _ := apperr.Details(err)
	if code != "invalid_pattern" {
		t.Fatalf("error code = %q, want invalid_pattern", code)
	}
}

func TestSearchHandlesContentLinesThatResembleDiffHeaders(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	path := "markers.txt"
	writeTestFile(t, root, path, "stable\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2024-01-01T00:00:00Z", "base")
	writeTestFile(t, root, path, "stable\n++RANDOM_ADD_MARKER\n")
	gitCommitAs(t, root, "Bob", "bob@example.com", "2024-02-01T00:00:00Z", "add marker")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	response, err := NewSearchService(repo).Search(context.Background(), searchOptions("*++RANDOM_ADD_MARKER*", "diff"))
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || response.Results[0].File.Path != path {
		t.Fatalf("header-like content was not searchable: %#v", response.Results)
	}
	if _, err := os.Stat(filepath.Join(root, path)); err != nil {
		t.Fatal(err)
	}
}
