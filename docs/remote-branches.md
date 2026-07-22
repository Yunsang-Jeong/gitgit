---
title: Remote Branch Visibility Plan
description: Settings의 remote 관리와 Commit의 remote branch 탐색을 분리하는 구현 계획
audience:
  - human
  - ai-agent
status: planned
document_type: plan
scope: remote-branches
last_updated: 2026-07-22
---

# Remote branch visibility

## 목표

Remote repository와 fetch된 remote-tracking branch를 Settings에서 확인하고, Commit의 Branch selector에서 필요한 remote branch만 선택해 history를 탐색한다. Remote branch를 본다는 이유로 local tracking branch나 worktree를 자동 생성하지 않는다.

## 제품 경계

- Settings는 remote와 branch의 상태를 관리하고, Commit은 history 탐색을 담당한다.
- Settings를 여는 것만으로 network fetch를 실행하지 않는다. 목록은 local `refs/remotes/*`를 기준으로 하며 갱신은 Commit의 명시적인 `Sync`를 사용한다.
- `All branches`의 기본 범위는 local branches와 remote default branch로 유지한다. 모든 remote branch를 자동 포함해 graph와 history 비용을 키우지 않는다.
- Remote branch를 선택하면 해당 ref 하나를 read-only history scope로 사용한다. Local branch 생성, checkout, push와 삭제는 별도 action 없이는 수행하지 않는다.

## 구현 단계

1. Backend에 remote별 branch 목록을 지연 조회하는 read API를 추가한다. Repository open 응답에는 큰 branch 목록을 넣지 않는다.
2. Settings의 `Remote badges`를 `Remotes` section으로 확장한다. Remote 이름, URL, default branch, fetch된 branch 개수와 검색 가능한 접힌 목록을 제공한다.
3. Commit Branch selector를 `Local branches`와 `Remote branches` group으로 분리한다. Remote 목록은 selector를 열거나 검색할 때 지연 조회한다.
4. 필요할 때만 Settings에 `Default branches only`, `Selected remote branches`, `All fetched remote branches` 포함 정책을 추가한다. 기본값은 `Default branches only`다.

## 완료 조건

- Remote가 많거나 branch가 많은 repository에서도 Repository open과 기본 Commit history 비용이 증가하지 않는다.
- Remote branch를 선택하면 정확한 ref history와 remote badge가 표시된다.
- Settings open, branch 조회와 remote branch 선택은 fetch, checkout 또는 local branch 생성을 유발하지 않는다.
- `All branches` 기본 결과와 기존 Sync의 fetch-and-prune 동작은 유지된다.
