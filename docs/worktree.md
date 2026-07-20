---
title: Worktree Module
description: Linked worktree의 상태 모델, 화면 구성, 선택과 안전한 제거 규칙
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: worktree
last_updated: 2026-07-20
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

Worktree는 다음 group으로 나뉜다.

- **Main**: Git이 보고한 primary worktree
- **Merged**: checked-out branch가 default branch에 merge된 linked worktree
- **Unmerged**: 아직 merge되지 않았거나 보호해야 하는 active worktree

각 card는 다음 정보를 표시한다.

- Branch 또는 detached 상태
- Absolute worktree path
- Short `HEAD`
- Clean 또는 Changes
- Locked 상태
- Sparse-checkout 활성화 여부
- Default, Merged, Unmerged 상태

Main worktree는 항상 먼저 표시하며 bulk-removal selection 대상이 아니다.

## Selection과 action

Card click 또는 checkbox로 linked worktree를 선택한다. Shift-click은 현재 정렬된 card 범위에 selection을 적용한다.

단일 선택 action:

- **View commits**: 해당 worktree를 active repository root로 열고 Commit 화면으로 이동한다.
- **Open in Finder**: worktree directory를 Finder에서 연다.
- **Open IDE**: 설정된 IDE로 directory를 연다.

Bulk action:

- **Remove worktrees & branches**: 선택 대상 전체가 검증을 통과한 경우에만 confirmation을 연다.
- **Clear merged worktrees**: 현재 제거 가능한 모든 linked worktree를 대상으로 같은 confirmation을 연다.

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

Remote fetch 결과가 필요한 최신 merge 상태는 사용자가 상단 `Sync`를 실행한 뒤 갱신된다. `Sync`는 remotes를 fetch하고 prune하지만 worktree나 branch를 자동 제거하지 않는다.

## Sparse-checkout

Worktree card는 sparse-checkout 활성화와 configured directory를 읽어 표시한다. 현재 UI는 sparse-checkout pattern을 만들거나 수정하지 않는다.

## 현재 제공하지 않는 것

- 새 linked worktree 생성
- Worktree path 이동
- Worktree에서 branch checkout
- Locked/dirty/unmerged worktree의 force removal
- Sparse-checkout enable/disable 또는 pattern mutation
- Remote branch 삭제
