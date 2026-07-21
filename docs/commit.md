---
title: Commit Module
description: Commit history 탐색, Preset, Inspector와 commit rewrite의 동작 및 안전 조건
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: commit
last_updated: 2026-07-21
---

# Commit Module

## 목적

Commit module은 선택한 worktree를 기준으로 Git history를 읽고, 별도의 checkout 없이 branch 범위를 비교하는 화면이다. 정확한 commit metadata와 changed files를 확인하는 read workflow가 기본이며, 조건을 만족할 때만 checked-out branch의 commit stack을 수정할 수 있다.

상단 control의 순서는 다음과 같다.

```text
Commits  [Worktree]  [Branch]  [Edit commits]
Preset   [My Jobs]  [3 Days]
```

## Worktree와 Branch 선택

두 selector는 역할이 다르다.

- **Worktree 선택**은 active repository root를 해당 checkout path로 바꾼다. Attached worktree는 checkout된 branch를, detached worktree는 `HEAD`를 초기 history scope로 사용한다.
- **Branch 선택**은 active worktree를 바꾸거나 branch를 checkout하지 않는다. Commit table이 읽는 revision scope만 바꾼다.
- **All branches**는 실제 `All`이라는 branch와 혼동하지 않도록 자연어 scope로 표시한다.

Branch dropdown은 다음 규칙을 사용한다.

1. Default branch
2. 현재 checked-out branch
3. 나머지 local branch

Dropdown에는 검색 input이 있으며 처음부터 모든 branch를 렌더링하지 않는다. 초기 25개를 노출하고 scroll 또는 keyboard navigation에 따라 다음 page를 보여준다. 다른 worktree가 사용 중인 branch와 worktree가 없는 branch도 badge로 구분한다.

## History loading

`All branches`에서는 선택 가능한 ref의 commit을 date order로 읽는다. Remote ref badge는 Settings의 URL mapping과 embedded provider icon을 사용한다.

Default branch가 아닌 local branch를 선택하면 초기 table은 다음 범위만 보여준다.

```text
branch HEAD → branch commits → default branch와의 branch point
```

Branch point 아래에는 `Load history before branch point`가 나타난다. 사용자가 이 경계를 명시적으로 넘긴 뒤에는 기존 near-bottom scroll loading이 다시 동작한다. 이 방식은 side branch를 열었을 때 default branch의 오래된 history가 먼저 화면을 채우지 않게 한다.

History batch size는 Settings에서 Automatic, 50, 100, 200, 500 commits 중 선택한다. Automatic은 table viewport의 약 두 배에 해당하는 행 수를 사용한다.

각 history page는 commit metadata를 한 번의 `git log`로 읽고, changed-file metadata도 전체 page를 한 번의 `git diff-tree --stdin` call로 보강한다. 같은 revision scope에서 다음 page를 읽을 때는 ref fingerprint가 유지되는 동안 total count, branch point와 branch list를 다시 계산하지 않는다. Scope나 commit 선택을 빠르게 바꾸면 이전 history와 commit-detail read를 cancel해 오래된 Git process가 background에 누적되지 않게 한다.

Preset이 활성화되면 이미 load된 commit만 검사하고 멈추지 않는다. 보이는 commit이 한 batch를 채우거나 revision scope의 끝에 도달할 때까지 history batch를 추가로 읽는다. Branch 조건은 새 batch의 branch membership을 먼저 보강한 뒤 같은 조건으로 평가한다. 사용자가 Preset을 선택한 경우에는 side branch의 branch-point 경계도 이 자동 확장을 막지 않는다.

추가 history를 읽는 동안 table은 활성 Preset 이름과 실제 적용된 조건, 목표 표시 개수, 현재 발견한 개수, 확인한 commit 수와 전체 scope를 표시한다. 탐색 방향은 선택 scope의 initial commit 방향으로 명시한다.

## Commit table과 Inspector

Commit table은 short SHA, subject, author, date를 표시하고 branch/ref badge는 subject 아래의 작은 보조 행에 둔다. Commit column에는 branch 정보를 섞지 않는다. Load된 일부 history나 Preset 결과만으로는 topology를 정확하게 표현할 수 없으므로 graph column은 제공하지 않는다. 행을 선택하면 Inspector가 다음 정보를 제공한다.

- Full commit hash, message, author, date와 refs
- Changed files list 또는 directory-first tree
- 선택한 file의 unified diff
- File path copy, Finder, terminal, IDE action
- Commit message, author, file path를 새 Search session으로 보내는 context action

Text hover는 interaction 종류를 구분한다. Cyan outline은 click 시 바로 copy되는 값이고, amber outline은 우클릭 context menu에서 copy, Search 추가, Finder/terminal 같은 action을 선택할 수 있는 값이다.

Remote badge는 GitHub, GitLab, Bitbucket, Azure DevOps, Codeberg, Gitea, Git, Cloud, self-hosted server와 generic remote icon을 application에 embedded SVG로 포함한다. Remote URL substring별 icon은 Settings에서 변경할 수 있다.

## Preset

Commit 화면은 임시 Filter composer를 제공하지 않고 Preset만 적용한다. 일회성 또는 복합 조건 탐색은 Search session에서 수행한다. Preset은 Commit module에만 적용되며 Search result에는 암묵적으로 적용되지 않는다.

Rule은 다음 field를 대상으로 한다.

- Branch
- Author
- Message
- Changed file
- Date

Action은 `Hide`, `Show`다. 여러 Show rule은 모두 만족해야 하고, Hide rule은 하나라도 만족하면 제외한다. `My Jobs`, `3 Days` 같은 Preset은 Settings에서 편집하며 `$me`와 `last:3d` 같은 값을 사용할 수 있다. 이전 settings에 남아 있는 `Highlight` rule은 load할 때 제외하며, 그 결과 유효한 rule이 하나도 없는 Preset도 표시하지 않는다.

## Edit commits 활성화 조건

`Edit commits`는 아래 조건을 모두 만족할 때 활성화된다.

1. Repository와 선택된 commit이 있다.
2. Project/worktree 전환 중이 아니다.
3. Active worktree가 detached `HEAD`가 아니라 local branch를 checkout하고 있다.
4. Branch scope가 `All branches`가 아니다.
5. 선택된 scope가 active worktree에서 실제로 checkout한 branch와 같다.

Worktree가 dirty한 것만으로 editing을 막지는 않는다. 대신 rewrite 결과와 기존 index/worktree 변경이 겹치는지 적용 전에 검사한다.

Branch dropdown에서 다른 branch를 선택해도 checkout이 일어나지 않으므로, 그 branch에만 속한 commit은 편집할 수 없다. 해당 branch를 checkout한 worktree를 먼저 선택해야 한다.

## Rewrite 범위와 안전 장치

선택한 commit부터 checked-out branch의 `HEAD`까지 first-parent chain을 oldest-first로 다시 만든다.

지원하는 변경:

- Commit 순서 변경
- Multiline commit message 수정
- 해당 commit이 변경한 regular text file의 content 수정 또는 삭제/복원

거부하는 조건:

- Root commit
- 선택 범위 안의 merge commit
- Checked-out branch의 first-parent history에 없는 commit
- 100개를 초과하는 commit range
- 2 MiB를 초과하는 file
- Binary file
- Symlink, gitlink 등 non-regular Git entry
- Rewrite 대상 tree와 기존 worktree/index 변경의 충돌
- 적용 도중 branch `HEAD`가 예상 값에서 변경된 경우

Rewrite는 temporary worktree에서 준비한다. 모든 replacement commit 생성과 local-change 검증이 끝난 뒤에만 branch를 이동한다. 이전 head는 다음 namespace 아래에 보존한다.

```text
refs/gitgit/backups/<branch>/<timestamp>
```

Default branch를 rewrite할 때는 별도 warning과 confirmation checkbox가 필요하다. 성공 후 commit hash가 바뀔 수 있지만 GitGit은 push 또는 force push를 자동 수행하지 않는다.

## 현재 제공하지 않는 것

- Branch selector를 통한 checkout
- Merge commit reorder
- Commit split/squash/fixup 전용 workflow
- Remote push와 force push
- Backup ref 정리 UI
