---
title: Remote Branch Visibility MVP
description: Local remote-tracking ref를 Settings, Commit과 Search에서 read-only로 탐색하는 동작과 경계
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: remote-branches
last_updated: 2026-08-07
---

# Remote branch visibility MVP

## 현재 상태

- Repository open은 remote 이름과 URL만 읽고 큰 branch 목록은 포함하지 않는다. Remote별 branch 목록은 local `refs/remotes/<remote>/*`에서 별도 API로 지연 조회한다.
- Settings의 `Remotes` section은 remote 이름, URL, default branch, fetched branch 개수와 검색 가능한 branch 목록을 표시한다. Provider icon mapping은 별도 `Remote appearance` section에서 관리한다.
- Commit과 Search의 Branch selector는 `Local branches`와 `Remote branches`를 분리하며, 선택한 remote branch는 `refs/remotes/<remote>/<branch>` exact ref로 유지한다.
- Commit table과 Branch selector는 remote ref의 badge와 provider icon을 표시한다. Inspector는 ref 이름을 text로 표시한다. Remote scope의 Commit `Edit Mode`는 비활성화된다.
- `Refresh`, `Sync`와 `Pull`은 in-memory remote catalog를 무효화한다. 같은 repository의 exact remote selection은 유지하고 새 local ref 상태로 다시 검증한다.

## 목적

Remote repository와 fetch된 remote-tracking branch를 Settings에서 확인하고, Commit과 Search의 Branch selector에서 필요한 remote branch만 선택해 history를 탐색한다. Remote branch를 본다는 이유로 local tracking branch나 worktree를 자동 생성하지 않는다.

## 제품 경계

- Settings는 local remote 상태를 설명하고, Commit과 Search는 history 탐색을 담당한다.
- 데이터 원본은 현재 repository에 이미 fetch된 local `refs/remotes/*`다. Settings open, branch 목록 조회와 remote branch 선택은 network fetch를 실행하지 않으며 갱신은 Commit의 명시적인 `Sync`를 사용한다.
- `All branches`의 기본 범위는 local branches와 remote default branch로 유지한다. 모든 remote branch를 자동 포함해 graph와 history 비용을 키우지 않는다.
- Remote branch를 선택하면 해당 exact ref 하나를 read-only history scope로 사용한다. Local tracking branch나 worktree 생성, checkout, commit rewrite, push와 remote branch 삭제는 수행하지 않는다.

## MVP UX

### Settings

- `Remotes` section은 현재 project의 remote 이름, URL, default branch와 fetched branch 개수를 표시한다.
- Remote별 branch 목록은 기본적으로 접고, 펼치면 검색과 점진적 표시를 제공한다. Badge icon mapping은 별도 `Remote appearance` section에 유지한다.
- Active repository 없음, remote 없음, loading, fetched branch 없음, read error와 검색 결과 없음 상태를 각각 구분한다.
- 한 remote의 조회 실패가 다른 remote 정보나 badge mapping을 숨기지 않는다. 오류가 난 remote에만 `Retry`를 제공한다.
- Settings open은 local ref read만 시작할 수 있으며 `Sync`를 호출하지 않는다.

### Branch selector

- Commit의 `All branches` 또는 Search의 `All refs`, detached `HEAD` 다음에 `Local branches`, `Remote branches` group을 분리해 표시한다.
- Local branches는 remote 조회 중이거나 실패해도 즉시 선택할 수 있다. Remote group만 loading, empty 또는 error 상태를 표시한다.
- Remote 목록은 selector를 처음 열거나 검색할 때 지연 조회한다. Local과 remote option은 각각 첫 25개를 표시하고 scroll 또는 keyboard navigation으로 다음 page를 노출한다.
- Remote option은 `origin/feature/name`처럼 remote 이름을 포함한 label과 provider icon을 사용한다. 같은 short branch name이 여러 remote에 있어도 구분할 수 있어야 한다.
- 선택한 remote ref가 Sync의 prune 뒤 사라져도 다른 branch로 조용히 변경하지 않는다. `Unavailable` 상태를 표시하고 사용자가 새 scope를 선택하게 한다.
- Commit의 `All branches`는 기존 범위를 유지하고 Search의 `All refs`는 모든 local·fetched remote ref를 유지한다. Exact remote ref는 두 workspace 모두에서 선택할 수 있다.
- Commit의 remote scope에서는 `Edit Mode`를 사용할 수 없으며 local branch checkout이나 생성 action을 제안하지 않는다.

## State 경계

- Remote branch catalog는 active repository와 함께 수명을 가진 in-memory read state다. Application setting이나 Git repository에 별도로 저장하지 않는다.
- Repository 또는 worktree 전환은 진행 중 remote read를 취소하고 이전 응답을 무시한다.
- Commit과 in-memory Search session의 remote selection은 같은 repository에서 history paging, view 이동과 picker 재open 동안 유지한다. Project 또는 worktree를 바꾸면 새 checkout context의 초기 scope를 사용한다.
- `Refresh`, `Sync` 또는 `Pull` 뒤에는 catalog를 무효화한다. Exact ref selection은 유지하되 새 local ref 상태로 다시 검증한다.
- Catalog read error는 ref가 없다는 뜻이 아니다. 성공한 catalog가 선택 ref의 부재를 확인했을 때만 `Unavailable`로 판정한다.

## 구현 구조

1. Backend `RemoteBranches` API는 remote 이름을 검증하고 local symbolic `HEAD`와 `refs/remotes/<remote>/*`만 읽어 default branch, exact ref와 count를 반환한다.
2. Frontend repository-scoped catalog는 remote별 loading, loaded, error와 branch 목록을 관리한다. Repository 전환 generation과 request ID가 stale response 적용을 막는다.
3. Settings의 `Remotes` section은 remote별 접힌 branch list, 검색, 25개 단위 `Show more`, empty/error와 `Retry`를 제공한다.
4. Commit과 Search가 공유하는 Branch selector는 local·remote option을 각각 25개씩 표시하고 scroll·keyboard pagination, exact ref label과 `Unavailable` 상태를 제공한다.
5. Remote side branch history는 같은 remote의 fetched default ref를 related scope로 사용한다. Default ref를 알 수 없거나 catalog read가 실패하면 related scope를 추측하지 않는다.

## MVP에서 제외하는 것

- Settings open 또는 branch 조회가 실행하는 implicit fetch
- Local tracking branch나 worktree 생성과 checkout
- Remote branch push, force push, rename 또는 삭제
- Remote branch 대상 commit rewrite
- `All branches`에 포함할 remote 범위를 사용자가 변경하는 policy UI
- Remote catalog와 branch selection의 disk persistence

## 보장 조건

- Remote가 많거나 branch가 많은 repository에서도 Repository open과 기본 Commit history 비용이 증가하지 않는다.
- Remote branch를 선택하면 정확한 ref history와 remote badge가 표시된다.
- Settings와 Branch selector가 remote 없음, loading, empty, error, no-match와 unavailable state를 구분한다.
- Settings open, branch 조회와 remote branch 선택은 fetch, checkout, local branch나 worktree 생성 또는 Git mutation을 유발하지 않는다.
- `All branches` 기본 결과와 기존 Sync의 fetch-and-prune 동작은 유지된다.
