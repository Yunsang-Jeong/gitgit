---
title: GitGit Overview
description: GitGit의 프로젝트 개요, 제품 방향, architecture와 현재 구현 상태
audience:
  - human
  - ai-agent
status: active
document_type: overview
scope: project
last_updated: 2026-07-20
---

# GitGit Overview

## 프로젝트 개요

GitGit은 Git repository의 작업 맥락을 읽기 쉽게 만드는 macOS desktop application이다. 여러 project와 linked worktree를 등록하고, worktree가 checkout한 branch와 별도로 history scope를 선택하며, commit과 changed files를 탐색한다.

핵심 관점은 다음과 같다.

- **Project**는 하나의 Git repository와 그 linked worktree 집합을 나타낸다.
- **Worktree**는 실제 filesystem checkout과 현재 `HEAD` 상태를 나타낸다.
- **Branch** selector는 checkout을 변경하지 않고 Commit 또는 Search가 읽을 revision scope만 바꾼다.
- **Commit**은 history 탐색과 좁은 범위의 rewrite를 담당한다.
- **Search**는 비용이 큰 history scan을 session 단위로 분리한다.

GitGit은 repository를 자체 형식으로 변환하거나 Git database를 대체하지 않는다. 모든 repository operation은 system Git을 통해 수행한다.

## 제품 방향

GitGit이 우선하는 방향은 세 가지다.

1. **Context preservation**: project, worktree, branch, commit의 차이를 UI에서 명시한다.
2. **Progressive loading**: branch option과 commit history를 한 번에 모두 렌더링하지 않는다.
3. **Guarded mutation**: read-only 탐색은 가볍게 제공하되, history rewrite와 worktree 제거는 대상과 실패 조건을 먼저 검증한다.

## 현재 구현 상태

### Commit

- Project, Worktree, Branch 선택과 `All branches` history를 지원한다.
- Default branch를 branch list의 첫 항목으로 배치한다.
- Side branch는 default branch와의 branch point까지만 먼저 보여주고, 사용자가 명시적으로 이전 history를 확장한 뒤 scroll loading을 재개한다.
- Commit Filter, preset, graph column, Inspector, changed-file list/tree와 diff를 제공한다.
- Checked-out local branch의 linear first-parent range에 한해 commit reorder, message edit, changed-file content edit를 제공한다.

상세 규칙은 [Commit](commit.md)을 따른다.

### Worktree

- Main, Merged, Unmerged group으로 linked worktree를 표시한다.
- Branch, path, head, dirty, locked, sparse-checkout, default merge 상태를 보여준다.
- Worktree를 Commit 화면의 active checkout으로 열거나 Finder/IDE에서 열 수 있다.
- 검증을 통과한 merged worktree와 local branch를 명시적 확인 후 제거할 수 있다.

상세 규칙은 [Worktree](worktree.md)를 따른다.

### Search

- Search는 Commit toolbar가 아니라 독립 workspace다.
- 여러 in-memory Search session을 만들고 Project, Worktree, Branch, query와 마지막 성공 결과를 각각 유지한다.
- `Message`, `DIFF`, `FILE` condition을 AND/OR로 조합하며 AND가 OR보다 먼저 평가된다.
- 결과는 file-level backend match를 commit 단위 한 행으로 합쳐 표시한다.

상세 규칙은 [Search](search.md)를 따른다.

## Architecture

| Layer | 책임 |
| --- | --- |
| `desktop/frontend/` | Svelte UI, 화면 상태, session state, 사용자 interaction |
| `desktop/` | Wails binding, application lifecycle, native dialog와 shell integration |
| `internal/desktop/` | Repository state, history/detail/search adapter, cache, project store, guarded mutation |
| `internal/app/` | Search expression, Git history와 worktree domain logic |
| `internal/gitexec/` | Argument-safe system Git process execution |

Backend는 한 시점에 하나의 active repository를 가진다. Frontend에서 project나 worktree를 전환하면 repository generation을 갱신해 오래된 비동기 응답이 새로운 화면 상태를 덮지 못하게 한다.

## 저장되는 상태

| 상태 | 위치 | 수명 |
| --- | --- | --- |
| Registered projects와 favorite | `~/Library/Application Support/GitGit/projects.json` | application 재시작 이후에도 유지 |
| Repository history metadata cache | `~/Library/Caches/com.wails.gitgit/cache-v1` | disposable, 삭제 후 재생성 |
| UI settings | WebView `localStorage`의 `gitgit.settings.v1` | local application data가 유지되는 동안 |
| Inspector pane width | WebView `localStorage`의 `gitgit.pane-widths.v1` | local application data가 유지되는 동안 |
| Search sessions | Frontend process memory | application 종료 시 삭제 |

`make uninstall`은 설치된 app bundle과 disposable cache를 제거하지만 registered-project list는 유지한다.

## Repository 선택

시작 시 repository는 다음 순서로 결정된다.

1. `--repository <path>` launch argument
2. Process working directory
3. Registered projects
4. Native repository chooser

Repository와 search option을 제어하는 `GITGIT_*` environment variable은 지원하지 않는다.

## 개발과 release 범위

```sh
make dev-browser # Wails browser bridge를 localhost:34116에서 실행
make check       # frontend test/check/build, Go race/vet, native build verification
make build       # local app bundle
make install     # $HOME/Applications/GitGit.app 교체
make test-random # deterministic randomized Go suites
```

Product code와 test 변경은 [Development Gate](development.md)의 browser-first 절차를 따른다. `make dev`는 `make dev-browser`의 alias이며, port가 충돌하면 `WAILS_DEVSERVER=localhost:<port>`로 바꿀 수 있다.

현재 product version은 `0.2.0`이다. Product version, timestamp build identifier, source revision은 status bar와 bundle metadata에 따로 기록된다.

Release artifact는 Apple Silicon과 macOS 11 이상을 대상으로 하며 local ad-hoc signing을 사용한다. Developer ID signing, notarization, Intel build와 external distribution은 현재 제공하지 않는다.

## 공통 경계

- Application-wide shortcut은 Settings의 `Command+,`만 유지한다.
- GitGit은 자동 push, force push 또는 remote branch 삭제를 수행하지 않는다.
- Worktree 생성·이동과 sparse-checkout mutation은 아직 제공하지 않는다.
- Search session persistence와 background search queue는 아직 제공하지 않는다.
- PR/MR/CI provider integration은 아직 제공하지 않는다.
