package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestGlobMatcherSemantics(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{name: "single star matches one segment", pattern: "*.go", value: "main.go", want: true},
		{name: "single star does not cross slash", pattern: "*.go", value: "cmd/main.go", want: false},
		{name: "globstar crosses slash", pattern: "**/*.go", value: "cmd/internal/main.go", want: true},
		{name: "globstar directory may be empty", pattern: "**/*.go", value: "main.go", want: true},
		{name: "glob is whole value", pattern: "fix", value: "prefix suffix", want: false},
		{name: "stars make text substring", pattern: "*fix*", value: "prefix suffix", want: true},
		{name: "question mark", pattern: "file?.go", value: "file1.go", want: true},
		{name: "character class", pattern: "file[0-9].go", value: "file7.go", want: true},
		{name: "negated character class", pattern: "file[!0-9].go", value: "filex.go", want: true},
		{name: "escaped metacharacter", pattern: `literal\*name`, value: "literal*name", want: true},
		{name: "unicode", pattern: "**/*검색*.md", value: "docs/검색-가이드.md", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher, err := newMatcher(test.pattern, "glob")
			if err != nil {
				t.Fatal(err)
			}
			if got := matcher(test.value); got != test.want {
				t.Fatalf("match(%q, %q) = %t, want %t", test.pattern, test.value, got, test.want)
			}
		})
	}
}

func TestRegexMatcherUsesGoSyntaxAndSubstringMatching(t *testing.T) {
	matcher, err := newMatcher(`TODO|FIXME`, "regex")
	if err != nil {
		t.Fatal(err)
	}
	if !matcher("prefix FIXME suffix") {
		t.Fatal("regex did not find a substring")
	}
	anchored, err := newMatcher(`^TODO$`, "regex")
	if err != nil {
		t.Fatal(err)
	}
	if anchored("prefix TODO suffix") {
		t.Fatal("anchored regex matched a substring")
	}
}

func TestTextGlobCrossesSlashesAndLines(t *testing.T) {
	matcher, err := compileSearchMatcher("*TODO*", "glob", "diff")
	if err != nil {
		t.Fatal(err)
	}
	if !matcher("url=https://example.com/a/b\nTODO: migrate endpoint\n") {
		t.Fatal("text glob did not cross slashes and line boundaries")
	}
}

func TestGlobMatcherRejectsMalformedPatterns(t *testing.T) {
	for _, pattern := range []string{"[", "[]", "[!]", `trailing\`} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := newMatcher(pattern, "glob"); err == nil {
				t.Fatal("malformed glob unexpectedly compiled")
			}
		})
	}
}

func TestFollowRenameDoesNotTreatCopiesAsRenames(t *testing.T) {
	matcher, err := newMatcher("destination.txt", "glob")
	if err != nil {
		t.Fatal(err)
	}
	tracked := map[string]bool{}
	copyChange := FileChange{Status: "C100", OldPath: "source.txt", Path: "destination.txt"}
	if !matchFileChange(copyChange, matcher, true, tracked) {
		t.Fatal("copy destination did not match its own file pattern")
	}
	if tracked["source.txt"] {
		t.Fatal("copy source was incorrectly added to rename lineage")
	}
}

func TestSearchCombinesSourcesWithoutDuplicateResults(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	const token = "UNION_RANDOM_TOKEN"
	writeTestFile(t, root, "src/UNION_RANDOM_TOKEN.txt", "value="+token+"\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2025-01-01T00:00:00Z", "feat: "+token)

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	progress := make([]SearchProgress, 0)
	response, err := NewSearchService(repo).SearchWithProgress(context.Background(), SearchOptions{
		Messages: []string{"*" + token + "*"},
		Diffs:    []string{"*" + token + "*"},
		Files:    []string{"**/*" + token + "*"},
		Engine:   "glob",
		Limit:    100,
		Context:  3,
	}, func(update SearchProgress) {
		progress = append(progress, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 {
		t.Fatalf("combined search count = %d, want 1: %#v", response.Count, response.Results)
	}
	if response.Scanned != 1 || len(progress) == 0 || progress[len(progress)-1] != (SearchProgress{Scanned: 1, Total: 1}) {
		t.Fatalf("search progress = %#v, response scanned = %d", progress, response.Scanned)
	}
	result := response.Results[0]
	if result.Message == "" || result.Date == "" || len(result.Files) != 1 {
		t.Fatalf("search did not reuse log and batch file metadata: %#v", result)
	}
	if !slices.Equal(response.Results[0].MatchSources, []string{"msg", "diff", "file"}) {
		t.Fatalf("sources = %v", response.Results[0].MatchSources)
	}
	if len(response.MessagePatterns) != 1 || len(response.DiffPatterns) != 1 || len(response.FilePatterns) != 1 || response.Engine != "glob" {
		t.Fatalf("incomplete response metadata: %#v", response)
	}
}

func TestSearchCombinesRepeatedPatternsWithinEachSource(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "src/alpha-one.txt", "signal=ALPHA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2025-01-01T00:00:00Z", "feat: ALPHA_MSG_TOKEN")
	writeTestFile(t, root, "docs/beta-two.md", "signal=BETA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Bob", "bob@example.com", "2025-02-01T00:00:00Z", "docs: BETA_MSG_TOKEN")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)
	tests := []struct {
		name    string
		options SearchOptions
		source  string
	}{
		{
			name: "messages",
			options: SearchOptions{
				Messages: []string{"*ALPHA_MSG_TOKEN*", "*BETA_MSG_TOKEN*"},
			},
			source: "msg",
		},
		{
			name: "diffs",
			options: SearchOptions{
				Diffs: []string{"*ALPHA_DIFF_TOKEN*", "*BETA_DIFF_TOKEN*"},
			},
			source: "diff",
		},
		{
			name: "files",
			options: SearchOptions{
				Files: []string{"**/alpha-*.txt", "**/beta-*.md"},
			},
			source: "file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.options
			options.Engine = "glob"
			options.Limit = 100
			options.Context = 3
			response, err := service.Search(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if response.Count != 2 {
				t.Fatalf("count = %d, want 2: %#v", response.Count, response.Results)
			}
			for _, result := range response.Results {
				if !slices.Equal(result.MatchSources, []string{test.source}) {
					t.Fatalf("sources = %v, want %q", result.MatchSources, test.source)
				}
			}
		})
	}
}

func TestSearchCombinesOrderedPredicatesWithAnd(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "src/alpha.txt", "signal=ALPHA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2025-01-01T00:00:00Z", "feat: ALPHA_MSG_TOKEN")
	writeTestFile(t, root, "docs/beta.md", "signal=BETA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Bob", "bob@example.com", "2025-02-01T00:00:00Z", "docs: BETA_MSG_TOKEN")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	response, err := NewSearchService(repo).Search(context.Background(), SearchOptions{
		Predicates: []SearchPredicate{
			{Source: "msg", Value: "*ALPHA_MSG_TOKEN*"},
			{Source: "file", Value: "**/*.txt", Join: "and"},
		},
		Engine: "glob", Limit: 100, Context: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || response.Results[0].File.Path != "src/alpha.txt" {
		t.Fatalf("AND search results = %#v", response.Results)
	}
	if !slices.Equal(response.Results[0].MatchSources, []string{"msg", "file"}) {
		t.Fatalf("sources = %v", response.Results[0].MatchSources)
	}
}

func TestSearchPredicateAndPrecedesOr(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "src/alpha.txt", "signal=ALPHA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2025-01-01T00:00:00Z", "feat: ALPHA_MSG_TOKEN")
	writeTestFile(t, root, "docs/beta.md", "signal=BETA_DIFF_TOKEN\n")
	gitCommitAs(t, root, "Bob", "bob@example.com", "2025-02-01T00:00:00Z", "docs: BETA_MSG_TOKEN")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	response, err := NewSearchService(repo).Search(context.Background(), SearchOptions{
		Predicates: []SearchPredicate{
			{Source: "msg", Value: "*ALPHA_MSG_TOKEN*"},
			{Source: "diff", Value: "*BETA_DIFF_TOKEN*", Join: "or"},
			{Source: "file", Value: "**/*.md", Join: "and"},
		},
		Engine: "glob", Limit: 100, Context: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 2 {
		t.Fatalf("precedence search count = %d, want 2: %#v", response.Count, response.Results)
	}
	paths := []string{response.Results[0].File.Path, response.Results[1].File.Path}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"docs/beta.md", "src/alpha.txt"}) {
		t.Fatalf("precedence search paths = %v", paths)
	}
}

func TestSearchPredicateParenthesesOverridePrecedence(t *testing.T) {
	predicates, err := compileSearchPredicates(SearchOptions{
		Predicates: []SearchPredicate{
			{Source: "msg", Value: "one", OpenGroups: 1},
			{Source: "file", Value: "two", Join: "or", CloseGroups: 1},
			{Source: "diff", Value: "three", Join: "and"},
		},
		Engine: "glob", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evaluateSearchExpression(predicates, []bool{true, false, false}) {
		t.Fatal("(true OR false) AND false should be false")
	}
	if !evaluateSearchExpression(predicates, []bool{false, true, true}) {
		t.Fatal("(false OR true) AND true should be true")
	}
}

func TestSearchPredicateRejectsUnbalancedParentheses(t *testing.T) {
	_, err := normalizedSearchPredicates(SearchOptions{Predicates: []SearchPredicate{
		{Source: "msg", Value: "one", OpenGroups: 1},
		{Source: "file", Value: "two", Join: "and"},
	}})
	if err == nil || !strings.Contains(err.Error(), "not balanced") {
		t.Fatalf("expected unbalanced parenthesis error, got %v", err)
	}
}

func TestSearchPredicatesWithoutJoinsPreserveLegacyOr(t *testing.T) {
	predicates, err := normalizedSearchPredicates(SearchOptions{Predicates: []SearchPredicate{
		{Source: "msg", Value: "one"},
		{Source: "file", Value: "two"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if predicates[0].Join != "" || predicates[1].Join != "or" {
		t.Fatalf("normalized joins = %#v", predicates)
	}
}

func TestFollowRenameTraversesCommitsExcludedByAuthorFilter(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	oldPath := "legacy/config.txt"
	newPath := "current/config.txt"
	writeTestFile(t, root, oldPath, "version=1\n")
	gitCommitAs(t, root, "Alice", "alice@example.com", "2024-01-01T00:00:00Z", "add legacy config")
	if err := os.MkdirAll(filepath.Join(root, "current"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, root, "mv", oldPath, newPath)
	gitCommitAs(t, root, "Bob", "bob@example.com", "2024-02-01T00:00:00Z", "rename config")
	writeTestFile(t, root, newPath, "version=2\n")
	gitCommitAs(t, root, "Carol", "carol@example.com", "2024-03-01T00:00:00Z", "update current config")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	response, err := NewSearchService(repo).Search(context.Background(), SearchOptions{
		Files:        []string{newPath},
		Engine:       "glob",
		FollowRename: true,
		Author:       "Alice",
		Limit:        100,
		Context:      3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || response.Results[0].File.Path != oldPath {
		t.Fatalf("filtered rename lineage = %#v", response.Results)
	}
}

func TestCompileSearchMatcherReportsSourceDetails(t *testing.T) {
	_, err := compileSearchMatcher("[", "regex", "diff")
	if err == nil {
		t.Fatal("invalid regex unexpectedly compiled")
	}
	code, _, details := apperr.Details(err)
	if code != "invalid_pattern" || details["source"] != "diff" || details["engine"] != "regex" {
		t.Fatalf("error = %q, details = %#v", code, details)
	}
}

func TestCompileSearchMatchersRejectsEmptyRepeatedPattern(t *testing.T) {
	_, err := compileSearchMatchers([]string{"*valid*", ""}, "glob", "msg")
	if err == nil {
		t.Fatal("empty repeated pattern unexpectedly compiled")
	}
	code, _, details := apperr.Details(err)
	if code != "invalid_pattern" || details["source"] != "msg" || details["pattern"] != "" {
		t.Fatalf("error = %q, details = %#v", code, details)
	}
}
