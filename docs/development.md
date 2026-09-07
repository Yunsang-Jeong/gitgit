---
title: Development and Browser Verification
description: Wails browser bridge를 우선하는 GitGit 개발과 검증 gate
audience:
  - human
  - ai-agent
status: active
document_type: gate
scope: development
last_updated: 2026-09-07
---

# Development and Browser Verification

## 목적

GitGit의 product code와 test는 실제 Wails binding을 통과하는 browser flow를 기준으로 개발하고 검증한다. Unit test, static build, raw Vite 화면만으로 작업 완료를 판정하지 않는다.

이 gate는 모든 동작을 browser automation으로 대체한다는 뜻이 아니다. 개발 초기에 실제 화면과 repository 상태를 확인하고, 구현 중 같은 Wails dev session으로 반복 검증하며, 자동화 test 뒤 영향을 받은 flow를 다시 확인하는 작업 규칙이다.

## 표준 진입점

Repository root에서 실행한다.

```sh
task dev:browser
```

기본 Wails devserver URL은 `http://localhost:34116`이다. Codex에서 작업할 때는 sidebar browser에서 이 URL을 연다. `task dev`와 `task dev-browser`는 같은 task의 alias다.

Wails browser bridge도 Go binding과 application lifecycle을 제공하기 위해 development app process를 실행한다. `task dev:browser`는 development 전용 environment를 전달해 native window를 처음부터 숨기므로 별도 app 창은 열리지 않는다. Process 자체를 종료하면 browser bridge도 함께 중단된다.

Port가 사용 중이면 명시적으로 바꾼다.

```sh
task dev:browser WAILS_DEVSERVER=localhost:34117
```

이 경우 browser에도 `http://localhost:34117`을 연다. Vite가 출력하는 `http://127.0.0.1:5173`은 사용하지 않는다. Raw Vite page에는 Wails runtime과 Go binding이 없으므로 실제 application flow를 검증할 수 없다.

이미 같은 repository를 위한 devserver가 실행 중이면 재사용한다. 불필요한 duplicate process를 만들지 않는다.

## Go Task와 toolchain

GitGit은 go-task 3.50 이상을 사용한다. Taskfile의 task는 macOS 전용이며, 다른 OS에서는 task body를 건너뛰지 않고 실행 전에 명확한 오류로 종료한다. Bundle platform은 `darwin/arm64`로 고정하지 않고 현재 macOS host의 architecture를 사용하며, `task check:native`도 같은 architecture를 검증한다.

Wails library와 versioned CLI는 함께 `v2.12.0`으로 pin한다. `v2.13.0`의 embedded browser-dev overlay는 Svelte 5 component를 legacy constructor 방식으로 실행해 reload 중 종료될 수 있으므로, upstream overlay가 Svelte 5 `mount` 또는 compatibility API로 수정되고 browser reload/reconnect와 native check를 다시 통과하기 전에는 한쪽 version만 올리지 않는다.

Taskfile은 시작 시 `npm`의 absolute path를 찾고 Wails child process의 `PATH` 앞에 그 directory를 전달한다. GUI shell처럼 login shell과 `PATH`가 다른 환경에서는 다음처럼 명시할 수 있다.

```sh
task build NPM=/opt/homebrew/bin/npm
```

`task frontend:install`과 `task frontend:build`는 `sources`/`generates` checksum으로 dependency와 Vite input이 바뀐 경우에만 다시 수행한다. `task bundle`은 이 frontend artifact를 준비한 뒤 Wails에는 `-s`로 넘기지만, release metadata와 signing이 현재 실행 시점의 값을 가져야 하므로 항상 새 bundle을 만든다. `-ldflags`는 Go version metadata에만 사용하며 `npm` 탐색이나 `PATH`를 변경하지 않는다.

Taskfile은 의존 task의 중복 실행을 막는 `run: once`, 실행 전 환경을 확인하는 `preconditions`, 성공/실패/중단 후 cleanup을 보장하는 `defer`, 이전 명령 습관을 위한 `aliases`를 사용한다. 일상적인 build, native check, install과 development server는 성공, 실패 또는 중단 시 `apps/desktop/build/bin`을 정리한다. Install 도중 사용하는 hidden staging app도 같은 방식으로 제거한다. 보존 가능한 app bundle 자체가 필요한 경우에만 `task bundle`을 명시적으로 실행한다. 따라서 Spotlight에서 실행 대상으로 선택할 app은 `$HOME/Applications/GitGit.app`이며, repository 내부의 `apps/desktop/build/bin/GitGit.app`은 `task bundle`을 실행한 경우에만 존재한다.

## Required gate

Product code 또는 test를 변경하는 작업은 다음 순서를 따른다.

1. 작업 초기에 `task dev:browser`를 시작하거나 기존 session을 재사용한다.
2. Wails devserver URL을 browser에서 열고 변경 대상 화면과 현재 상태를 확인한다.
3. 구현 중 hot reload를 사용해 영향을 받은 flow를 반복 확인한다.
4. Targeted test와 `task check` 등 변경 위험에 맞는 자동화 검증을 실행한다.
5. 자동화 검증 뒤 같은 browser flow를 다시 실행하고 결과를 기록한다.

다음 변경은 이 gate의 대상이다.

- Svelte component, style, interaction, routing과 화면 state
- Wails-bound Go method와 frontend/backend contract
- Repository, worktree, history, search, editing처럼 UI가 노출하는 domain behavior
- Product behavior를 새로 정의하거나 변경하는 test
- Loading, cancellation, generation, session처럼 화면 전환에 따라 달라지는 비동기 behavior
- Git process batching, scope cache와 progress event처럼 read 성능이나 process lifetime을 바꾸는 behavior

## 통과 기준

작업 범위에 맞게 아래 조건을 확인한다.

- Wails devserver URL이 열리고 `Desktop bridge unavailable` fallback이 아니라 실제 Wails binding을 사용한다.
- 변경 대상 flow를 실제 repository 또는 목적에 맞는 fixture로 실행한다.
- Normal state와 변경에 직접 관련된 empty, loading, error, disabled state를 확인한다.
- Project, worktree, branch, Search session 전환이 관련된 경우 선택과 결과가 올바르게 함께 바뀐다.
- Browser 확인 뒤에도 targeted test와 정적 검증이 통과한다.
- History/Search 최적화는 결과 동등성뿐 아니라 page당 `diff-tree --stdin` batch 수, 중복 count/metadata command 제거와 이전 request cancellation test를 함께 통과한다.
- Repository open과 history read를 바꾸는 작업은 `task check:performance`의 load budget을 함께 통과한다.
- 최종 보고에 browser URL, 확인한 flow, 사용한 repository 또는 fixture, 자동화 검증 결과를 남긴다.

다음 항목만으로는 gate를 통과하지 않는다.

- `npm run build` 또는 `task check`만 실행
- Raw Vite URL에서 정적 UI만 확인
- Mock screenshot이나 DOM snapshot만 확인
- Unit test만 통과하고 Wails-exposed flow를 실행하지 않음

## 개발용 project 등록

Project picker의 `Register Git repository`는 native directory dialog를 연다. 검증 대상 repository를 자주 바꾸는 개발 중에는 이를 우회할 수 있다.

```sh
task dev:project -- /absolute/path/to/repo
task dev:project PROJECT=subgit            # ROOT_DIR 기준 상대 경로도 가능
```

이 task는 app이 쓰는 것과 같은 `projects.json`에 직접 기록하며, `canonicalProjectRoot`와 같은 규칙(absolute, symlink 해석, clean)으로 경로를 정규화한다. 이미 등록된 repository는 다시 추가하지 않고, Git repository가 아니면 거부한다.

`ProjectStore`는 접근할 때마다 file을 다시 읽으므로 app을 재시작할 필요는 없다. 다만 frontend는 project 목록을 시작 시 한 번만 가져오고 `Refresh`는 repository 상태만 갱신하므로, picker에 반영하려면 **page를 새로고침**한다.

## Large repository load budget

Read 성능은 작은 fixture로는 드러나지 않는다. `internal/desktop`의 `TestLargeRepositoryLoadBudget`은 14만 commit 규모의 실제 저장소에서 repository open과 history read의 wall-clock을 재고 예산을 넘으면 실패한다.

```sh
task fixture:large          # 없을 때만 clone한다 (kubernetes, 약 1.6 GB)
task check:performance
```

Fixture는 기본적으로 `$XDG_CACHE_HOME/gitgit/test-fixtures/kubernetes`(기본 `~/.cache/gitgit/`)에 두며 `GITGIT_LARGE_FIXTURE`로 다른 경로를 지정할 수 있다.

이 gate는 `task check`에 포함하지 않고 opt-in으로 둔다. 시간 측정에 의존하므로 machine이 바쁠 때 실행되면 변경과 무관한 이유로 실패하기 때문이다. `GITGIT_LARGE_FIXTURE`가 없거나 fixture가 없으면 실패가 아니라 skip한다.

예산은 목표가 아니라 상한이며, 기준 machine에서 관측된 가장 느린 값의 약 2배로 잡았다. Commit마다 process를 하나 더 띄우거나 batch를 잃는 것 같은 algorithmic regression은 잡되, 평범한 hardware 편차로는 흔들리지 않게 하기 위한 값이다.

| 구간 | 예산 | 환경 변수 |
| --- | --- | --- |
| Repository open | 2000 ms | `GITGIT_OPEN_BUDGET_MS` |
| History 첫 page (100 commit) | 6000 ms | `GITGIT_FIRST_HISTORY_BUDGET_MS` |
| History 이후 page | 3000 ms | `GITGIT_FURTHER_HISTORY_BUDGET_MS` |

측정은 3회 반복해 **최솟값**을 사용한다. Noise는 시간을 더하기만 하므로, 상한을 판단할 때는 최솟값이 안정적인 답이다. 예산을 올릴 때는 반드시 근거가 되는 측정을 함께 남긴다.

## 예외와 실패 처리

문서만 변경한 작업은 frontmatter, link, `git diff --check` 검증으로 대신할 수 있다. Product behavior를 설명하는 문서를 code나 test와 함께 바꾸는 경우에는 예외가 아니다.

직접 대응하는 UI가 없는 internal behavior는 가장 가까운 Wails-exposed flow를 실행한다. 현재 UI로 도달할 수 없다면 targeted test를 실행하고, 최종 보고에 browser에서 검증할 수 없었던 범위와 이유를 명시한다. 이를 browser gate 통과로 기록하지 않는다.

Wails devserver 또는 sidebar browser를 사용할 수 없으면 정적 Vite 화면으로 조용히 대체하지 않는다. 가능한 자동화 검증은 계속 진행하되 browser gate는 `blocked` 또는 `partial`로 보고한다.

## 완료 기록

최종 보고에는 다음 정보를 짧게 남긴다.

```text
Browser gate: passed | partial | blocked
URL: http://localhost:34116
Flow: 확인한 화면과 interaction
Repository/fixture: 사용한 대상
Observed: 핵심 결과
Automated checks: 실행한 command와 결과
```

이 browser bridge는 development 전용이다. Release artifact의 native WebView와 signing 검증은 `task check`와 `task check:native`(`task check-native` alias)가 별도로 담당하며 검증용 bundle은 완료 후 제거한다.
