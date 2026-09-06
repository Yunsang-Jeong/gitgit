---
title: GitGit Overview
description: GitGit의 프로젝트 개요, 제품 방향, architecture와 현재 구현 상태
audience:
  - human
  - ai-agent
status: active
document_type: overview
scope: project
last_updated: 2026-09-07
---

# GitGit Overview

## 프로젝트 개요

GitGit은 Git repository의 작업 맥락을 읽기 쉽게 만드는 macOS desktop application과 read-only VS Code extension이다. Desktop에서는 여러 project와 linked worktree를 등록하고, worktree가 checkout한 branch와 별도로 history scope를 선택하며, commit과 changed files를 탐색한다. VS Code에서는 같은 system Git domain을 사용해 현재 editor의 blame, Search와 commit change range를 확인한다.

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
- Commit Preset, Inspector, changed-file list/tree와 diff를 제공한다. Preset은 보이는 행이 한 batch를 채울 때까지 history를 점진적으로 확장하며 탐색 조건과 범위를 표시한다.
- Checked-out local branch의 linear first-parent range에 한해 commit reorder와 message, author, author date edit를 제공한다. 그 commit이 이미 바꾼 file은 Edit Mode에서 내용 치환·삭제·복원할 수 있으며, replacement commit에 `rewritten from:` provenance를 남기는 opt-in을 제공한다.

상세 규칙은 [Commit](commit.md)을 따른다.

### Worktree

- Main, Merged, Unmerged group으로 linked worktree를 표시한다.
- Branch, path, head, dirty, locked, sparse-checkout, default merge 상태를 보여준다.
- Worktree를 Commit 화면의 active checkout으로 열거나 Finder/IDE에서 열 수 있다.
- 검증을 통과한 merged worktree와 local branch를 명시적 확인 후 제거할 수 있다.

상세 규칙은 [Worktree](worktree.md)를 따른다.

### Search

- Search는 Commit toolbar가 아니라 독립 workspace다.
- 여러 in-memory Search session을 명시적으로 만들고 alias, Project, Worktree, Branch, query, 마지막 실행 시각과 마지막 성공 결과를 각각 유지하거나 삭제한다.
- `Message`, `DIFF`, `FILE` condition을 AND/OR로 조합하고 인접한 행을 선택해 visual parenthesis group을 만들며, 생성된 expression을 함께 표시한다.
- Backend는 file별 Boolean expression을 평가한 뒤 unique commit result로 합치며, 실제 `matched files`와 commit 전체 `changed files`를 분리한다. Desktop은 compact session sidebar와 Inspector를 함께 제공해 선택한 commit의 changed files와 diff를 조사한다.

상세 규칙은 [Search](search.md)를 따른다.

### Remote branches

- Settings에서 현재 repository의 remote와 local `refs/remotes/*`에 이미 존재하는 remote-tracking branch를 확인한다.
- Commit과 Search의 Branch selector는 local branch와 remote branch를 분리해 보여 주고, 선택한 exact remote ref 하나를 read-only history scope로 사용한다.
- Remote 목록은 Settings open 또는 Branch selector open에서 지연 조회한다. Settings open, remote branch 조회와 선택은 network fetch, checkout, local tracking branch나 worktree 생성, commit rewrite, push 또는 삭제를 수행하지 않는다.
- `Refresh`, `Sync`와 `Pull` 뒤에는 in-memory remote catalog를 무효화하고 필요한 목록을 다시 읽는다. Network를 통한 remote ref 갱신은 Commit의 명시적인 `Sync`만 담당한다.
- `All branches`의 기존 local branch와 matching remote default branch 범위는 유지한다. 모든 fetched remote branch를 자동으로 포함하지 않는다.

상세 동작과 상태·오류 경계는 [Remote branches](remote-branches.md)를 따른다.

### Visual Studio Code

- Local VS Code에서 active file의 line blame과 file history를 조회한다.
- 현재 repository에서 GitGit Search를 native Tree View와 QuickInput으로 실행하고, commit-first 결과의 matched/changed file을 editor navigation에 연결한다.
- 선택한 commit의 read-only revision snapshot을 열고 target range와 deletion anchor를 editor decoration으로 표시한다. File 전체 삭제는 parent snapshot을 연다.
- Extension은 target별 bundled helper를 사용하며 Desktop 설치나 실행에 의존하지 않는다.
- v0.1 VSIX target은 `darwin-arm64`, `darwin-x64`, `win32-x64`로 한정한다.

기능, protocol, 지원 환경과 release invariant는 [Visual Studio Code Extension](vscode.md)을 따른다.

## Architecture

배포 가능한 두 client는 `apps/` 아래에서 분리하고, 공유 Git domain과 platform adapter는 root Go module의 `internal/`에 둔다.

```text
apps/
├── desktop/                 # Wails application과 Svelte frontend
└── vscode/                  # 독립 VSIX package
    ├── src/
    │   ├── extension.ts     # composition root
    │   ├── editor/          # blame, history, revision snapshot과 commit highlight
    │   ├── helper/          # helper client와 protocol parser
    │   ├── search/          # expression, state, native Tree View와 QuickInput
    │   └── shared/          # repository identifier와 path 규칙
    └── tests/               # src feature 경계를 그대로 mirror
cmd/
└── gitgit-vscode-helper/    # bundled helper executable entrypoint
internal/
├── app/                     # 공유 Search와 worktree domain
├── gitexec/                 # argument-safe system Git execution
├── desktop/                 # Desktop-specific adapter와 persistence
└── vscodehelper/            # read-only JSON-RPC server implementation
testdata/
├── search-contract/         # Desktop과 VSIX의 Search contract
└── vscode-contract/         # Extension과 helper의 protocol contract
```

`apps/desktop`과 `apps/vscode`는 서로 설치나 실행을 요구하지 않는다. 두 client가 공유하는 것은 `internal/app`, `internal/gitexec`의 Git 동작과 versioned contract이며 UI, process lifecycle, artifact와 update channel은 각각 소유한다.

Backend는 한 시점에 하나의 active repository를 가진다. Frontend에서 project나 worktree를 전환하면 repository generation을 갱신해 오래된 비동기 응답이 새로운 화면 상태를 덮지 못하게 한다.

History, branch membership과 commit detail은 화면별로 `latest request wins` 규칙을 사용한다. 같은 화면에서 새 요청이 시작되면 frontend가 이전 응답을 무시하는 데서 그치지 않고, backend가 이전 context를 취소해 실행 중인 system Git process도 종료한다. Repository tree는 changed directory 여러 개를 병렬 확장하므로 directory read끼리는 취소하지 않고 repository 전환 시 함께 취소한다.

Read 성능은 system Git 경계를 유지하면서 process 수와 중복 parsing을 줄이는 방식으로 관리한다.

- History page의 changed-file metadata는 commit마다 `git show`를 실행하지 않고 최대 500개 commit을 한 번의 `git diff-tree --stdin`으로 읽는다.
- 같은 history scope의 total, branch point와 branch list는 ref fingerprint가 바뀌기 전까지 page 사이에서 재사용한다.
- Search는 commit metadata를 한 번의 `git log`에서 읽고 changed-file metadata를 500-commit batch로 처리한다. Scope count와 result별 metadata를 별도 Git process로 다시 읽지 않으며, DIFF condition이 없으면 result table이 사용하지 않는 unified diff도 만들지 않는다.
- Search progress는 bounded event로 전달하지만 성공 전 partial result로 마지막 성공 결과를 교체하지 않는다.

전체 worktree를 recursive filesystem watcher로 감시하지 않는다. 큰 repository에서 watcher 자체가 높은 비용과 platform별 누락 위험을 만들 수 있으므로, ref-dependent cache는 request 경계의 ref fingerprint로 무효화하고 worktree 상태는 명시적인 repository refresh에서 다시 읽는다.

## 저장되는 상태

| 상태 | 위치 | 수명 |
| --- | --- | --- |
| Registered projects와 favorite | `~/Library/Application Support/GitGit/projects.json` | application 재시작 이후에도 유지 |
| Repository history metadata cache | `~/Library/Caches/com.wails.gitgit/cache-v1` | disposable, 삭제 후 재생성 |
| UI settings | WebView `localStorage`의 `gitgit.settings.v1` | local application data가 유지되는 동안 |
| Inspector pane width | WebView `localStorage`의 `gitgit.pane-widths.v1` | local application data가 유지되는 동안 |
| Search sessions | Frontend process memory | application 종료 시 삭제 |

`task uninstall`은 설치된 app bundle과 disposable cache를 제거하지만 registered-project list는 유지한다.

등록 project를 해제해도 Git repository나 worktree는 삭제하지 않는다. 현재 열려 있는 project를 해제한 경우에도 현재 worktree는 다른 project를 선택할 때까지 열린 상태로 유지한다.

Settings의 `Remove unavailable`은 경로가 사라졌거나 더 이상 해당 project의 Git worktree가 아닌 registered-project entry만 목록에서 제거한다. Git 실행 환경 오류나 취소가 발생하면 목록을 변경하지 않으며, 이 작업도 repository나 worktree를 삭제하지 않는다.

Settings의 `Discover recursively`는 먼저 사용자가 지정한 directory 아래만 재귀 탐색한다. Native folder chooser 또는 직접 입력한 path를 사용하며, 발견한 Git project root만 등록할 뿐 repository나 worktree를 변경하지 않는다.

## Repository 선택

시작 시 repository는 다음 순서로 결정된다.

1. `--repository <path>` launch argument
2. Process working directory
3. Registered projects
4. Native repository chooser

Repository와 search option을 제어하는 `GITGIT_*` environment variable은 지원하지 않는다.

## 개발과 release 범위

```sh
task dev:browser # Wails browser bridge를 localhost:34116에서 실행
task check       # frontend test/check/build, Go race/vet, native build verification
task build       # local app compile/sign 검증 후 임시 bundle 제거
task bundle      # apps/desktop/build/bin/GitGit.app을 명시적으로 보존
task install     # $HOME/Applications/GitGit.app 교체 후 중간 artifact 제거
task test:random # deterministic randomized Go suites
```

Product code와 test 변경은 [Development Gate](development.md)의 browser-first 절차를 따른다. `task dev`와 `task dev-browser`는 `task dev:browser`의 alias이며, port가 충돌하면 `WAILS_DEVSERVER=localhost:<port>`로 바꿀 수 있다. Taskfile은 frontend 입력에는 checksum cache를 적용하지만 release bundle은 매번 새로 수행한다.

현재 product version은 `0.2.0`이다. Product version, timestamp build identifier, source revision은 status bar와 bundle metadata에 따로 기록된다.

Desktop release artifact는 현재 build를 실행하는 Mac의 architecture와 macOS 11 이상을 대상으로 하며 local ad-hoc signing을 사용한다. Universal binary, Developer ID signing, notarization과 external distribution은 현재 제공하지 않는다.

VS Code extension v0.1은 같은 extension ID와 version으로 `darwin-arm64`, `darwin-x64`, `win32-x64` platform VSIX를 각각 만든다. 각 artifact에는 target과 일치하는 helper 하나만 포함한다. Generic/Web, Linux, Windows ARM64, WSL과 Remote extension host는 v0.1 release 대상이 아니다. 상세 package와 native verification 기준은 [Visual Studio Code Extension](vscode.md)의 release invariant를 따른다.

## 공통 경계

- Application-wide shortcut은 Settings의 `Command+,`만 유지한다. Commit과 Search의 결과 목록은 focus가 목록 안에 있을 때만 동작하는 방향키 이동을 제공하며, 이는 application-wide shortcut으로 취급하지 않는다.
- GitGit은 자동 push, force push 또는 remote branch 삭제를 수행하지 않는다.
- Worktree 생성·이동과 sparse-checkout mutation은 아직 제공하지 않는다.
- Search session persistence와 background search queue는 아직 제공하지 않는다.
- PR/MR/CI provider API integration은 아직 제공하지 않는다. VS Code blame의 local metadata 기반 review link 추론은 network integration으로 취급하지 않는다.
- VS Code extension과 helper는 read-only이며 repository mutation이나 network operation을 수행하지 않는다.
