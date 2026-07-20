#!/bin/sh

set -e

fixture_version="gitgit-subgit-v4"
previous_fixture_version="gitgit-subgit-v3"
fixture_commits="100"
previous_fixture_commits="1000"
fixture_branches="12"
fixture_worktrees="12"

usage() {
	echo "usage: setup.sh <create|reset> <repository-path>" >&2
	exit 2
}

if [ "$#" -ne 2 ]; then
	usage
fi

action=$1
repo=$2
marker="$repo/.git/gitgit-subgit-fixture"
worktree_root="${repo}-worktrees"
worktree_marker="$worktree_root/.gitgit-subgit-worktrees"

die() {
	echo "subgit fixture: $1" >&2
	exit 1
}

marker_version() {
	marker_path=${1:-$marker}
	IFS= read -r version < "$marker_path" || version=""
	printf '%s' "$version"
}

fixture_matches() {
	expected_version=$1
	expected_commits=$2
	expected_branches=$3
	[ -f "$marker" ] || return 1
	[ "$(marker_version)" = "$expected_version" ] || return 1
	[ "$(git -C "$repo" rev-list --count HEAD 2>/dev/null)" = "$expected_commits" ] || return 1
	[ "$(git -C "$repo" branch --format='%(refname:short)' 2>/dev/null | wc -l | tr -d ' ')" = "$expected_branches" ] || return 1
	[ "$(git -C "$repo" worktree list --porcelain 2>/dev/null | grep -c '^worktree ')" = "$fixture_worktrees" ] || return 1
	[ "$(git -C "$repo" log -1 --format=%s 2>/dev/null)" = "release: fixture history v2" ] || return 1
	[ -z "$(git -C "$repo" status --porcelain 2>/dev/null)" ] || return 1
	[ -f "$worktree_marker" ] || return 1
	[ "$(marker_version "$worktree_marker")" = "$expected_version" ] || return 1
	return 0
}

fixture_is_current() {
	fixture_matches "$fixture_version" "$fixture_commits" "$fixture_branches"
}

fixture_is_previous() {
	fixture_matches "$previous_fixture_version" "$previous_fixture_commits" "$fixture_branches"
}

commit_fixture() {
	commit_directory=$1
	author_name=$2
	author_email=$3
	commit_date=$4
	commit_message=$5

	env \
		GIT_AUTHOR_NAME="$author_name" \
		GIT_AUTHOR_EMAIL="$author_email" \
		GIT_AUTHOR_DATE="$commit_date" \
		GIT_COMMITTER_NAME="GitGit Fixture Bot" \
		GIT_COMMITTER_EMAIL="fixture@gitgit.local" \
		GIT_COMMITTER_DATE="$commit_date" \
		git -C "$commit_directory" commit --quiet -m "$commit_message"
}

create_side_branch() {
	git -C "$repo" switch --quiet --create scenario-side main
	mkdir -p "$repo/side"
	printf '%s\n' "SIDE_BRANCH_ONLY" > "$repo/side/only.txt"
	git -C "$repo" add --all
	commit_fixture "$repo" "Eve Example" "eve@example.com" \
		"2026-04-01T10:00:00+09:00" \
		"feat: SIDE_BRANCH_ONLY"
	git -C "$repo" switch --quiet main
}

create_fixture() {
	if [ -e "$repo" ]; then
		if fixture_is_current; then
			echo "subgit fixture ready: $repo ($fixture_commits commits)"
			return
		fi
		if fixture_is_previous; then
			die "$repo uses the previous 1,000-commit fixture; run 'make subgit-reset' once to rebuild the 100-commit fixture"
		fi
		die "$repo already exists or differs from the managed fixture; run 'make subgit-reset' to rebuild it"
	fi

	mkdir -p "$repo"
	git init --quiet -b main "$repo"
	git -C "$repo" config commit.gpgsign false
	git -C "$repo" config tag.gpgsign false
	git -C "$repo" config core.autocrlf false
	printf '%s\n' "$fixture_version" > "$marker"

	mkdir -p "$repo/a/b/c" "$repo/app" "$repo/legacy"
	printf '%s\n' \
		"# GitGit subgit fixture" \
		"" \
		"Deterministic history for GitGit integration tests." > "$repo/README.md"
	printf '%s\n' \
		"fixture_path=lowercase" \
		"lineage=case-only-rename" > "$repo/a/b/c/d.txt"
	printf '%s\n' \
		"batch_size=8" \
		"mode=literal" > "$repo/app/config.txt"
	printf '%s\n' "legacy fixture content" > "$repo/legacy/obsolete.txt"
	git -C "$repo" add --all
	commit_fixture "$repo" "Alice Example" "alice@example.com" \
		"2024-01-10T09:00:00+0900" \
		"chore(fixture): seed repository"

	printf '%s\n' \
		"matcher=glob" \
		"pattern=FIXTURE_GLOB_ALPHA" > "$repo/app/search.txt"
	git -C "$repo" add --all
	commit_fixture "$repo" "Bob Example" "bob@example.com" \
		"2024-03-15T10:30:00+0900" \
		"feat(search): add glob tokenizer [GG-101]"

	mkdir -p "$repo/internal"
	printf '%s\n' \
		"escaped_star=FIXTURE_ESCAPED_STAR" \
		"regex_anchor=^history$" > "$repo/internal/parser.txt"
	git -C "$repo" add --all
	commit_fixture "$repo" "Alice Example" "alice@example.com" \
		"2024-06-20T14:00:00+0900" \
		"fix(parser): handle escaped star"

	mkdir -p "$repo/docs"
	printf '%s\n' \
		"# Historical lookup" \
		"" \
		"Search by period, author, message, diff, and filename." > "$repo/docs/history.md"
	printf '%s\n' \
		"Fixture notes with a space in the filename." > "$repo/docs/release notes.md"
	git -C "$repo" rm --quiet legacy/obsolete.txt
	git -C "$repo" add --all
	commit_fixture "$repo" "Carol Example" "carol@example.com" \
		"2025-02-11T08:15:00+0900" \
		"docs: document historical lookup"

	mkdir -p "$repo/internal/search"
	git -C "$repo" mv app/search.txt internal/search/pipeline.txt
	git -C "$repo" add --all
	commit_fixture "$repo" "Bob Example" "bob@example.com" \
		"2025-05-05T16:45:00+0900" \
		"refactor(search): move pattern pipeline"

	git -C "$repo" mv -f a/b/c/d.txt a/b/c/D.txt
	git -C "$repo" add --all
	commit_fixture "$repo" "Dana Example" "dana@example.com" \
		"2025-08-21T11:20:00+0900" \
		"fix(paths): preserve case-only rename"

	printf '%s\n' \
		"batch_size=64" \
		"mode=literal" \
		"marker=PERF_BATCH_64" > "$repo/app/config.txt"
	printf '%s\n' \
		"fixture_path=uppercase" \
		"lineage=case-only-rename" \
		"post_rename=CASE_FOLLOW_CURRENT" > "$repo/a/b/c/D.txt"
	git -C "$repo" add --all
	commit_fixture "$repo" "Alice Example" "alice@example.com" \
		"2026-01-12T13:10:00+0900" \
		"perf(diff): batch changed-line scans"

	mkdir -p "$repo/.timeline"
	checkpoint=1
	while [ "$checkpoint" -le 92 ]; do
		checkpoint_id=$(printf '%04d' "$checkpoint")
		case $((checkpoint % 10)) in
			0) checkpoint_topic="renderer cache" ;;
			1) checkpoint_topic="graph lanes" ;;
			2) checkpoint_topic="author index" ;;
			3) checkpoint_topic="path metadata" ;;
			4) checkpoint_topic="revision window" ;;
			5) checkpoint_topic="branch badges" ;;
			6) checkpoint_topic="worktree status" ;;
			7) checkpoint_topic="filter presets" ;;
			8) checkpoint_topic="diff preview" ;;
			9) checkpoint_topic="project registry" ;;
		esac
		case $((checkpoint % 6)) in
			0) checkpoint_author="Timeline Bot"; checkpoint_email="timeline@gitgit.local" ;;
			1) checkpoint_author="Alice Example"; checkpoint_email="alice@example.com" ;;
			2) checkpoint_author="Bob Example"; checkpoint_email="bob@example.com" ;;
			3) checkpoint_author="Carol Example"; checkpoint_email="carol@example.com" ;;
			4) checkpoint_author="Dana Example"; checkpoint_email="dana@example.com" ;;
			5) checkpoint_author="Fixture Robot"; checkpoint_email="robot@gitgit.local" ;;
		esac
		checkpoint_timestamp=$((1769907600 + checkpoint * 60))
		printf '%s\n' \
			"checkpoint=$checkpoint_id" \
			"topic=$checkpoint_topic" \
			"seed=gitgit-subgit-v2" > "$repo/.timeline/checkpoint-$checkpoint_id.txt"
		git -C "$repo" add --all
		commit_fixture "$repo" "$checkpoint_author" "$checkpoint_email" \
			"$checkpoint_timestamp +0900" \
			"chore(timeline): checkpoint $checkpoint_id $checkpoint_topic"
		checkpoint=$((checkpoint + 1))
	done

	printf '%s\n' \
		"matcher=glob" \
		"pattern=FIXTURE_GLOB_ALPHA" \
		"fallback=FIXTURE_REGEX_OMEGA" > "$repo/internal/search/pipeline.txt"
	printf '%s\n' \
		"# Fixture history v2" \
		"" \
		"- Added regex fallback coverage." > "$repo/CHANGELOG.md"
	git -C "$repo" add --all
	commit_fixture "$repo" "Bob Example" "bob@example.com" \
		"2026-03-03T17:30:00+0900" \
		"release: fixture history v2"

	git -C "$repo" tag fixture-v2

	mkdir -p "$worktree_root"
	printf '%s\n' "$fixture_version" > "$worktree_marker"

	search_tree="$worktree_root/feature search UI"
	git -C "$repo" worktree add --quiet -b feature/search-ui "$search_tree" main
	mkdir -p "$search_tree/ui"
	printf '%s\n' "drawer=compact" "preset=enabled" > "$search_tree/ui/search-pane.txt"
	git -C "$search_tree" add --all
	commit_fixture "$search_tree" "Erin Example" "erin@example.com" \
		"2026-03-10T09:20:00+0900" \
		"feat(ui): preview compact search drawer"

	sync_tree="$worktree_root/worktree-sync"
	git -C "$repo" worktree add --quiet -b feature/worktree-sync "$sync_tree" main
	mkdir -p "$sync_tree/internal/worktree"
	printf '%s\n' "fetch_all=true" "fast_forward=warn" > "$sync_tree/internal/worktree/sync.txt"
	git -C "$sync_tree" add --all
	commit_fixture "$sync_tree" "Frank Example" "frank@example.com" \
		"2026-03-11T10:15:00+0900" \
		"feat(worktree): trace remote synchronization"

	rename_tree="$worktree_root/rename tracking"
	git -C "$repo" worktree add --quiet -b bugfix/rename-tracking "$rename_tree" main
	printf '%s\n' "rename_audit=enabled" > "$rename_tree/a/b/c/rename-audit.txt"
	git -C "$rename_tree" add --all
	commit_fixture "$rename_tree" "Grace Example" "grace@example.com" \
		"2026-03-12T14:40:00+0900" \
		"fix(rename): retain case-only lineage"
	printf '%s\n' "uncommitted_fixture_change=true" >> "$rename_tree/a/b/c/rename-audit.txt"

	release_tree="$worktree_root/release-0.2"
	git -C "$repo" worktree add --quiet -b release/0.2 "$release_tree" main
	printf '%s\n' "release=0.2" "status=staging" > "$release_tree/RELEASE.txt"
	git -C "$release_tree" add --all
	commit_fixture "$release_tree" "Hana Example" "hana@example.com" \
		"2026-03-13T11:25:00+0900" \
		"release: prepare 0.2 validation"
	git -C "$repo" worktree lock --reason "fixture maintenance" "$release_tree"

	sparse_tree="$worktree_root/sparse index"
	git -C "$repo" worktree add --quiet -b experiment/sparse-index "$sparse_tree" main
	mkdir -p "$sparse_tree/experiments"
	printf '%s\n' "index=sparse" "cone=true" > "$sparse_tree/experiments/sparse-index.txt"
	git -C "$sparse_tree" add --all
	commit_fixture "$sparse_tree" "Ian Example" "ian@example.com" \
		"2026-03-14T08:35:00+0900" \
		"experiment: prototype sparse index"
	git -C "$sparse_tree" sparse-checkout set internal docs

	detached_tree="$worktree_root/review detached"
	git -C "$repo" worktree add --quiet --detach "$detached_tree" fixture-v2^

	docs_tree="$worktree_root/history notes"
	git -C "$repo" worktree add --quiet -b docs/history-notes "$docs_tree" main

	graph_tree="$worktree_root/graph cache"
	git -C "$repo" worktree add --quiet -b perf/graph-cache "$graph_tree" main

	author_tree="$worktree_root/author index"
	git -C "$repo" worktree add --quiet -b chore/author-index "$author_tree" main

	registry_tree="$worktree_root/project registry"
	git -C "$repo" worktree add --quiet -b feature/project-registry "$registry_tree" main

	density_tree="$worktree_root/commit density"
	git -C "$repo" worktree add --quiet -b experiment/commit-density "$density_tree" main

	create_side_branch

	echo "subgit fixture created: $repo ($fixture_commits main commits, $fixture_branches branches, $fixture_worktrees worktrees)"
}

reset_fixture() {
	if [ ! -f "$marker" ]; then
		die "refusing to reset $repo because it is not a managed subgit fixture"
	fi
	current_version=$(marker_version)
	case "$current_version" in
		gitgit-subgit-v1)
			if [ -e "$worktree_root" ]; then
				die "refusing to migrate $repo because $worktree_root already exists without a v2 fixture marker"
			fi
			;;
		"$previous_fixture_version"|"$fixture_version")
			if [ ! -f "$worktree_marker" ] || [ "$(marker_version "$worktree_marker")" != "$fixture_version" ]; then
				if [ "$(marker_version "$worktree_marker")" != "$current_version" ]; then
					die "refusing to reset $worktree_root because its fixture marker is missing or unknown"
				fi
			fi
			;;
		*)
			die "refusing to reset $repo because its fixture marker is unknown"
			;;
	esac
	case "$repo" in
		""|"/"|"$HOME")
			die "refusing to reset unsafe path: $repo"
			;;
	esac
	case "$worktree_root" in
		""|"/"|"$HOME")
			die "refusing to reset unsafe worktree path: $worktree_root"
			;;
	esac
	if [ "$current_version" = "$previous_fixture_version" ] || [ "$current_version" = "$fixture_version" ]; then
		rm -rf "$repo" "$worktree_root"
	else
		rm -rf "$repo"
	fi
	create_fixture
}

case "$action" in
	create)
		create_fixture
		;;
	reset)
		reset_fixture
		;;
	*)
		usage
		;;
esac
