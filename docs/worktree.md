---
title: Worktree Module
description: Linked worktree의 상태 모델, 화면 구성, 선택과 생성·이동·sparse-checkout·제거 규칙
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: worktree
last_updated: 2026-09-07
---

# Worktree Module

## 목적

Worktree module은 하나의 Git repository에 연결된 checkout들을 작업 단위로 비교한다. Directory를 직접 찾아다니지 않고 어떤 branch가 어디에서 사용 중인지, merge와 정리 가능 상태가 어떤지 확인하는 것이 목적이다.

## Worktree와 Branch 관계

일반적인 attached 상태에서는 하나의 local branch를 동시에 하나의 linked worktree만 checkout할 수 있으므로 worktree와 checked-out branch가 1:1처럼 보인다. 그러나 이는 영구적인 identity가 아니다.

- Worktree는 다른 branch로 전환될 수 있다.
- Worktree는 detached `HEAD`가 될 수 있다.
- Branch는 worktree 없이 존재할 수 있다.
- Worktree가 제거되어도 다른 branch와 repository history는 남을 수 있다.

따라서 GitGit은 worktree path와 branch name을 별도 상태로 다룬다. Branch selector는 history scope이고, Worktree selector는 실제 checkout context다.

## 화면 구성

상단은 36px 한 줄 section bar를 사용한다. `WORKTREES`와 detected default branch만 왼쪽에 표시하고 worktree 수는 하단 status bar에 맡긴다. `Actions`와 화면에 축약해 표시하는 `Clear merged`는 26px compact control이며, 후자는 접근성 이름으로 `Clear merged worktrees`를 유지한다.

Worktree는 다음 group으로 나뉜다.

- **Main**: Git이 보고한 primary worktree
- **Merged**: checked-out branch가 default branch에 merge된 linked worktree
- **Unmerged**: 아직 merge되지 않았거나 보호해야 하는 active worktree

각 card는 다음 정보를 표시한다.

Card는 Commit 화면과 같은 32px rhythm을 사용한다. Summary, path/status, per-card action을 각각 32px rail로 구성하고 `Commits`, `Finder`, `IDE`는 rail 안의 26px compact control로 정렬한다. 선택 상태는 Commit row와 같은 blue fill 및 왼쪽 accent를 사용한다.

- Branch 또는 detached 상태
- Absolute worktree path
- Short `HEAD`
- Clean 또는 Changes
- Locked 상태
- Sparse-checkout 활성화 여부
- Default 상태

Path는 앞의 directory 부분만 ellipsis로 줄이고 마지막 segment는 항상 끝까지 보여 준다. 여러 worktree가 같은 parent directory를 공유할 때 card를 구분하는 정보는 경로의 끝에 있기 때문이다.

Merged와 Unmerged는 card badge로 반복하지 않는다. Card가 이미 해당 group 안에 있으므로 badge는 group에서 알 수 없는 정보인 `Default`, `Locked`, `Sparse`만 표시한다.

Main worktree는 항상 먼저 표시한다. Selection 자체는 가능하지만 bulk-removal 대상은 아니며, 제거는 이름으로 거부한다. Sparse-checkout은 primary checkout에서 가장 필요하므로 선택을 막지 않는다.

## Selection과 action

Card click 또는 checkbox로 linked worktree를 선택한다. Shift-click은 현재 정렬된 card 범위에 selection을 적용한다.

각 card의 footer에는 그 worktree만 대상으로 하는 **Commits**, **Finder**, **IDE**를 둔다. 선택 없이 바로 실행할 수 있으므로 단일 worktree를 열 때 상단 menu를 거치지 않는다.

상단 header의 **New worktree**는 선택과 무관하게 새 linked worktree를 만든다.

단일 선택 action(상단 `Actions` menu):

- **View commits**: 해당 worktree를 active repository root로 열고 Commit 화면으로 이동한다.
- **Open in Finder**: worktree directory를 Finder에서 연다.
- **Move…**: worktree directory를 다른 위치로 옮긴다.
- **Sparse-checkout…**: 유지할 directory를 고른다.

Bulk action:

- **Remove worktrees & branches**: 선택 대상 전체가 검증을 통과한 경우에만 confirmation을 연다.
- **Clear merged worktrees**: 현재 제거 가능한 모든 linked worktree를 대상으로 같은 confirmation을 연다.

파괴적 action은 실제로 실행할 수 있을 때만 danger 색을 사용한다. 대상이 없어 disabled인 `Clear merged worktrees`는 중립 색으로 표시해 빈 상태에서 시선을 끌지 않는다.

## 제거 가능 조건

Worktree와 그 local branch를 함께 제거하려면 다음 조건을 모두 만족해야 한다.

1. Main worktree가 아니다.
2. 현재 active worktree가 아니다.
3. Detached worktree가 아니며 local branch가 있다.
4. Default branch를 checkout하고 있지 않다.
5. Locked 상태가 아니다.
6. Worktree가 clean 상태다.
7. Branch가 detected default branch에 merge되어 있다.

Frontend 확인은 action availability를 설명하기 위한 1차 guard다. Backend는 실제 mutation 직전에 repository state와 대상 path/branch를 다시 검증한다. 하나라도 실패하면 선택 전체를 제거하지 않는다.

Removal은 되돌리기 어려운 operation이다. Confirmation에는 제거할 branch와 absolute path를 다시 보여준다. GitGit은 unmerged branch, dirty worktree 또는 locked worktree를 강제로 제거하지 않는다.

## Default branch와 merge 판정

Default branch는 remote symbolic ref, primary worktree branch와 deterministic local-ref fallback을 이용해 탐지한다. 각 attached worktree의 branch가 default branch에 merge되었는지는 Git ancestry를 기준으로 계산한다.

Remote update는 Commit 화면의 상단 `Sync`와 `Pull`에서만 명시적으로 실행한다. `Sync`는 remotes를 fetch하고 prune하지만 worktree나 branch를 자동 제거하지 않는다. `Pull`은 current tracking branch만 fast-forward하며 merge commit을 만들지 않는다. Worktree 화면에서는 실수로 network operation을 시작하지 않도록 두 action을 disabled 상태로 표시하며, local repository state는 `Refresh`로 다시 읽을 수 있다.

## 생성

`New worktree`는 기존 parent directory와 새 directory 이름 하나로 destination을 정한다. 이름은 path separator, newline, `.`/`..`, 선행 dash를 허용하지 않으며 이미 존재하는 경로나 등록된 worktree와 포함관계인 경로는 거부한다. Branch 이름을 고르면 directory 이름이 따라오되(`feature/thing` → `feature-thing`) 사용자가 직접 입력하면 그 값을 유지한다.

Checkout 방식은 셋 중 하나다.

- **New branch**: 새 branch를 만든다. 이미 있는 이름과 `check-ref-format`을 통과하지 못하는 이름은 거부한다. Start point는 선택 사항이다.
- **Existing branch**: 다른 worktree가 이미 checkout한 branch는 목록에 넣지 않는다.
- **Detached**: 지정한 revision을 detached HEAD로 checkout한다.

생성은 network를 사용하지 않는다. Remote update는 Commit 화면의 `Sync`와 `Pull` 소관이며, 다른 worktree를 만드는 동작이 지금 보고 있는 branch를 fast-forward하는 부수효과를 만들지 않는다.

## 이동

`Move…`는 checkout을 다른 위치로 옮기고 Git registration을 갱신한다. Branch, commit, local change는 그대로다. Destination 규칙은 생성과 같으며 현재 위치와 같은 경로는 거부한다.

다음 worktree는 이동하지 않는다.

- Main worktree
- 지금 보고 있는 worktree — Service가 repository handle을 그 경로에 고정하므로, 옮기면 이후 모든 command가 사라진 경로를 가리킨다. Main으로 전환한 뒤 옮긴다.
- Locked worktree, disk에서 사라진 worktree, local change가 있는 worktree

Dialog는 `from → to`를 함께 보여 주며 그것이 확인 역할을 한다. 제거와 달리 없어지는 것이 없으므로 별도 confirmation modal을 두지 않는다.

## Sparse-checkout

Worktree card는 sparse-checkout 활성화와 configured directory를 읽어 표시한다. `Sparse-checkout…`은 그 선택을 편집한다.

GitGit은 **cone mode만** 관리한다. 이미 non-cone sparse checkout이 켜진 worktree는 이유를 표시하고 편집하지 않는다. Bare worktree, disk에서 사라진 worktree, locked worktree, unborn HEAD도 같은 방식으로 제외한다.

Directory tree는 그 worktree의 HEAD에서 한 level씩 읽는다. 큰 repository의 모든 directory를 미리 가져올 수 없기 때문이고, backend가 새 directory를 검증하는 대상도 같은 tree다. Cone mode에서 선택한 directory는 그 아래 전부를 포함하므로 하위 directory는 따로 보내지 않는다.

선택 변화는 다음으로 옮긴다.

| 변화 | 동작 |
| --- | --- |
| Sparse가 꺼져 있고 선택이 있음 | `set` |
| 추가만 있음 | `expand` |
| 제거만 있음 | `contract` |
| 추가와 제거가 함께 | `set` — 원자적으로 적용되며 dirty-path 검사를 함께 수행한다 |

Directory를 숨기는 변화(`contract`와 혼합 `set`)만 두 번 확인한다. 나머지는 파일을 되살리는 방향이므로 확인을 요구하지 않는다. 빈 선택은 "root file만"과 "전체 checkout 복구" 사이에서 모호하므로 plan 결과가 되지 않으며, 전체 복구는 `Disable sparse-checkout`으로 명시한다.

선택 밖에 local change가 있는 경로를 숨기려 하면 backend가 그 경로를 이름으로 알려 주며 거부한다.

## 현재 제공하지 않는 것

- Worktree에서 branch checkout
- Locked/dirty/unmerged worktree의 force removal
- Non-cone sparse-checkout 관리
- 생성 시 remote branch tracking(`--track`)과 remote sync
- Worktree lock/unlock과 prune
- 생성·이동·sparse-checkout의 일괄 실행
- 이동한 worktree를 대상으로 하던 Search session의 자동 재연결
- Remote branch 삭제
