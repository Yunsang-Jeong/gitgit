---
title: Development and Browser Verification
description: Wails browser bridge를 우선하는 GitGit 개발과 검증 gate
audience:
  - human
  - ai-agent
status: active
document_type: gate
scope: development
last_updated: 2026-07-20
---

# Development and Browser Verification

## 목적

GitGit의 product code와 test는 실제 Wails binding을 통과하는 browser flow를 기준으로 개발하고 검증한다. Unit test, static build, raw Vite 화면만으로 작업 완료를 판정하지 않는다.

이 gate는 모든 동작을 browser automation으로 대체한다는 뜻이 아니다. 개발 초기에 실제 화면과 repository 상태를 확인하고, 구현 중 같은 Wails dev session으로 반복 검증하며, 자동화 test 뒤 영향을 받은 flow를 다시 확인하는 작업 규칙이다.

## 표준 진입점

Repository root에서 실행한다.

```sh
make dev-browser
```

기본 Wails devserver URL은 `http://localhost:34116`이다. Codex에서 작업할 때는 sidebar browser에서 이 URL을 연다. `make dev`는 같은 target의 alias다.

Port가 사용 중이면 명시적으로 바꾼다.

```sh
make dev-browser WAILS_DEVSERVER=localhost:34117
```

이 경우 browser에도 `http://localhost:34117`을 연다. Vite가 출력하는 `http://127.0.0.1:5173`은 사용하지 않는다. Raw Vite page에는 Wails runtime과 Go binding이 없으므로 실제 application flow를 검증할 수 없다.

이미 같은 repository를 위한 devserver가 실행 중이면 재사용한다. 불필요한 duplicate process를 만들지 않는다.

## Required gate

Product code 또는 test를 변경하는 작업은 다음 순서를 따른다.

1. 작업 초기에 `make dev-browser`를 시작하거나 기존 session을 재사용한다.
2. Wails devserver URL을 browser에서 열고 변경 대상 화면과 현재 상태를 확인한다.
3. 구현 중 hot reload를 사용해 영향을 받은 flow를 반복 확인한다.
4. Targeted test와 `make check` 등 변경 위험에 맞는 자동화 검증을 실행한다.
5. 자동화 검증 뒤 같은 browser flow를 다시 실행하고 결과를 기록한다.

다음 변경은 이 gate의 대상이다.

- Svelte component, style, interaction, routing과 화면 state
- Wails-bound Go method와 frontend/backend contract
- Repository, worktree, history, search, editing처럼 UI가 노출하는 domain behavior
- Product behavior를 새로 정의하거나 변경하는 test
- Loading, cancellation, generation, session처럼 화면 전환에 따라 달라지는 비동기 behavior

## 통과 기준

작업 범위에 맞게 아래 조건을 확인한다.

- Wails devserver URL이 열리고 `Desktop bridge unavailable` fallback이 아니라 실제 Wails binding을 사용한다.
- 변경 대상 flow를 실제 repository 또는 목적에 맞는 fixture로 실행한다.
- Normal state와 변경에 직접 관련된 empty, loading, error, disabled state를 확인한다.
- Project, worktree, branch, Search session 전환이 관련된 경우 선택과 결과가 올바르게 함께 바뀐다.
- Browser 확인 뒤에도 targeted test와 정적 검증이 통과한다.
- 최종 보고에 browser URL, 확인한 flow, 사용한 repository 또는 fixture, 자동화 검증 결과를 남긴다.

다음 항목만으로는 gate를 통과하지 않는다.

- `npm run build` 또는 `make check`만 실행
- Raw Vite URL에서 정적 UI만 확인
- Mock screenshot이나 DOM snapshot만 확인
- Unit test만 통과하고 Wails-exposed flow를 실행하지 않음

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

이 browser bridge는 development 전용이다. Release artifact의 native WebView와 signing 검증은 `make check`와 `make check-native`가 별도로 담당한다.
