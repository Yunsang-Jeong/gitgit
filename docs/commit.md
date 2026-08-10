---
title: Commit Module
description: Commit history 탐색, Preset, Inspector와 commit rewrite의 동작 및 안전 조건
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: commit
last_updated: 2026-08-08
---

# Commit Module

## 목적

Commit module은 선택한 worktree를 기준으로 Git history를 읽고, 별도의 checkout 없이 branch 범위를 비교하는 화면이다. 정확한 commit metadata와 changed files를 확인하는 read workflow가 기본이며, Edit Mode에서는 현재 보고 있는 history를 그대로 local rewrite draft로 전환하고, Review와 승인 뒤에만 local rewrite를 수행한다.

상단 control의 순서는 다음과 같다.

```text
[Worktree: main]  [Branch: All branches]  [My Jobs]  [3 Days]  [✎ Edit Mode]  [▱ Finder]  [⌘ Terminal]  [↗ IDE]  ─ workspace full width
Inspector  [Changes]  [Files]
```

## Worktree와 Branch 선택

두 selector는 역할이 다르다.

- **Worktree 선택**은 active repository root를 해당 checkout path로 바꾼다. Attached worktree는 checkout된 branch를, detached worktree는 `HEAD`를 초기 history scope로 사용한다.
- **Branch 선택**은 active worktree를 바꾸거나 branch를 checkout하지 않는다. Commit table이 읽는 revision scope만 바꾼다.
- Worktree와 Branch selector, Preset button, **✎ Edit Mode**, **▱ Finder**, **⌘ Terminal**, **↗ IDE**는 Inspector 위까지 이어지는 36px workspace-wide toolbar 한 줄에 둔다. Preset과 worktree action 묶음은 Branch dropdown의 우측에 놓는다. Inspector와 Commit table은 이 toolbar 바로 아래에서 시작한다. Selector와 Preset button은 26px 높이이며, Project·Worktree·Branch dropdown의 검색 행과 선택 행은 32px 높이를 공유한다. Dropdown의 primary text, path와 status badge는 이 compact row에 맞는 type scale을 사용하고 badge 묶음은 한 줄을 유지한다. Preset button은 각각 최소·최대 폭 안에서 label을 ellipsis 처리한다. 뒤 세 action은 선택된 commit이나 file이 아니라 현재 선택된 worktree root를 연다.
- **All branches**는 실제 `All`이라는 branch와 혼동하지 않도록 자연어 scope로 표시한다.

Branch dropdown은 다음 규칙을 사용한다.

1. Default branch
2. 현재 checked-out branch
3. 나머지 local branch

Dropdown에는 검색 input이 있으며 처음부터 모든 branch를 렌더링하지 않는다. 초기 25개를 노출하고 scroll 또는 keyboard navigation에 따라 다음 page를 보여준다. 다른 worktree가 사용 중인 branch와 worktree가 없는 branch도 badge로 구분한다.

## History loading

History는 commit을 author-date order로 읽되 topology 제약을 유지해 parent가 child보다 먼저 나타나지 않게 한다. 화면에 표시하는 author date를 우선하므로 merge된 side lineage의 오래된 commit이 최신 날짜 사이에 통째로 끼는 현상을 줄인다. `All branches`에서는 선택 가능한 ref를 함께 읽는다. Remote ref badge는 Settings의 URL mapping과 embedded provider icon을 사용한다.

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

Commit table은 별도 header와 cell border 없이 branch/ref badge, graph, subject(첫 줄) 순서의 32px compact row로 표시한다. Commit hash와 date는 table에서 제거하고 Inspector에서 확인한다. Author는 message cell의 오른쪽 끝에 muted text로 표시하며, 자세한 email은 hover title과 Inspector에서 확인한다. Message cell은 최대 폭을 두어, 넓은 창에서 subject와 author 사이가 화면 폭만큼 벌어지지 않게 한다.

Commit table은 ARIA table로 노출한다. Scroll container와 row wrapper는 `presentation`으로 두어 row가 rowgroup에 직접 속하게 하고, 날짜 구분선도 하나의 cell을 가진 row로 표시한다. Row 자체가 선택과 context menu를 담당하므로 message cell에는 중첩 button을 두지 않는다. Row는 roving tabindex를 사용해 목록 전체가 tab stop 하나만 차지하며, 목록 안에서는 `↑`/`↓`, `PageUp`/`PageDown`, `Home`/`End`로 이동하고 이동한 row를 즉시 선택한다. `Enter`와 `Space`는 현재 row를 선택한다. Search 결과 table도 같은 규칙을 따른다. Branch/ref badge는 graph 왼쪽의 고정 column에 두고 모든 primary badge를 같은 폭으로 표시하며 긴 이름은 ellipsis로 줄인다. Primary badge의 text, border와 background tint는 해당 commit node의 graph lane 색상을 사용한다. 단, exact local default branch의 primary graph lane은 graph palette와 구분되는 white beam treatment(밝은 백청색과 은은한 glow)을 사용하며 `origin/<default>`가 primary head인 remote default lane에는 적용하지 않는다. Badge 자체는 이 glow를 쓰지 않는다. 고정 폭 badge에 gradient와 glow를 함께 적용하면 text input처럼 읽히므로, local default badge도 다른 badge와 같은 flat tint에 밝은 백청색 border와 text만 사용한다. 표시할 ref가 여러 개면 default branch를 우선한 첫 badge와 중립색의 별도 고정 폭 `+N` badge만 노출한다. Remote badge를 숨기는 설정이 적용된 ref는 `N`에 포함하지 않는다. Merge source branch의 ref가 삭제됐어도 merge message에서 이름을 복원할 수 있으면 second-parent tip commit 한 곳에 같은 크기와 graph lane 색상의 historical branch badge를 표시한다. 해당 commit에 실제 ref가 남아 있으면 실제 ref badge를 우선한다. Graph는 load된 commit의 parent 관계를 표현하고 default branch lane을 가장 왼쪽에 유지한다. All branches에서는 local default branch가 뒤처져 있어도 `origin/<default>`를 primary head로 유지하고, local branch scope에서는 exact local head를 사용한다. Remote default branch의 node와 first-parent line은 첫 palette blue로 고정하며 side lineage의 생성, collapse 또는 merge가 이 색상을 덮어쓰지 않는다. Merge 교차점에서도 side path가 default path를 가리지 않도록 default path를 마지막에 조금 더 두껍게 그린다. Side path는 default node 중심에서 색이 바뀌는 것처럼 보이지 않도록 node의 오른쪽 경계에 연결한다. 종료된 lane의 오른쪽 lane은 빈 공간을 남기지 않고 왼쪽으로 collapse한다. Lane color는 현재 x 위치가 아니라 active commit lineage에 속하므로, 같은 lineage가 빈 lane을 메우기 위해 왼쪽으로 이동해도 색상을 유지한다. Commit table 폭에 비례해 실제 lane을 6개에서 10개까지 표시하되, message 영역을 더 침범하지 않도록 10개를 상한으로 둔다. 이 범위의 lane color는 반복하지 않는다. 상한을 넘는 topology는 마지막 중립색 dashed lane 하나로 collapse한다. 실제 lane과 overflow lane 사이의 연결선은 branch 색과 중립색 사이의 gradient를 사용해 색상 전환 위치를 보존한다.

Graph는 row마다 SVG를 만들지 않고 visible history 전체 높이를 소유하는 하나의 responsive-width SVG overlay로 그린다. Scroll은 이 overlay를 table content와 함께 이동시킬 뿐 draw를 다시 실행하지 않는다. Table 또는 Inspector 폭이 바뀌면 표시 가능한 lane 수와 graph width를 다시 계산한다. History page나 Preset 결과가 바뀌면 visible commit topology를 먼저 완성한 뒤 overlay 하나를 교체해 기존 row와 새 row가 서로 다른 lane 좌표를 잠시 사용하는 상태를 만들지 않는다. Preset으로 중간 commit이 숨겨지면 hidden chain을 가장 가까운 visible ancestor로 collapse한 뒤 graph를 계산한다. Default chain의 first parent가 side lane에 이미 예약되어 있어도 lane 0으로 재배치하고 기존 side lane을 합류시켜 MR 사이의 default line을 유지한다.

Commit 날짜 구분선은 현재 local calendar를 기준으로 최근 7일은 일별, 같은 해의 그 이전 history는 월별, 이전 해는 연별로 표시한다. 월별 label은 해당 월의 `1.`로, 연별 label은 해당 연도의 `1. 1.`로 정규화한다. Topology 제약으로 날짜 bucket이 과거로 내려갔다가 최신으로 되돌아오면 되돌아온 날짜 구분선을 다시 표시해 뒤의 commit이 오래된 구분선에 속한 것처럼 보이지 않게 한다. Separator가 추가한 높이는 graph 좌표에도 반영하고 lane은 separator 아래까지 연속해서 그린다. 행을 선택하면 Inspector가 다음 정보를 제공한다.

- Full commit hash, commit message, author, date와 refs. Message는 첫 줄을 heading으로, 나머지 body와 trailer는 줄바꿈을 유지한 muted 본문 block으로 분리해 위계를 유지한다. Body가 길면 자체 scroll을 사용하며, 복사는 원문 전체를 대상으로 한다.
- 직접 가리키는 ref가 없는 merge second-parent commit은 merge source를 `Branch`로 표시하고, 이를 복원할 근거가 없을 때만 local branch containment를 `Branches`로 표시
- Changed files list 또는 directory-first tree. List row는 directory 부분만 ellipsis로 줄이고 file 이름은 항상 끝까지 보여 준다. Search match badge가 붙는 row도 같은 규칙을 사용해 이름이 badge 아래로 가려지지 않게 한다.
- 선택한 file의 unified diff
- File path copy, Finder, terminal action
- Commit message, author, file path를 새 Search session으로 보내는 context action

Text hover는 interaction 종류를 구분한다. Cyan outline은 click 시 바로 copy되는 값이고, amber outline은 우클릭 context menu에서 copy, Search 추가, Finder/terminal 같은 action을 선택할 수 있는 값이다. Author는 별도 interaction layer 없이 선택 가능한 일반 text로 표시한다. Review link는 ref badge와 같은 compact control 높이를 사용한다.

Remote badge는 GitHub, GitLab, Bitbucket, Azure DevOps, Codeberg, Gitea, Git, Cloud, self-hosted server와 generic remote icon을 application에 embedded SVG로 포함한다. Remote URL substring별 icon은 Settings에서 변경할 수 있다. Merge commit의 PR/MR link는 upstream remote를 우선해 만들며, SSH 또는 Git transport URL의 port는 browser URL로 옮기지 않아 HTTPS 기본 port로 연다.

## Remote updates

Commit 화면 우측 상단에는 `Sync`와 `Pull`을 함께 둔다. `Sync`는 모든 remote를 fetch하고 prune하지만 checked-out branch를 움직이지 않는다. `Pull`은 current tracking branch만 `git pull --ff-only`로 fast-forward한다. Diverged history는 merge commit을 만들지 않고 error로 남긴다. 두 action은 동시에 실행할 수 없고 Worktree와 Search 화면에서는 disabled 상태로 표시한다.

## Preset

Commit 화면은 임시 Filter composer를 제공하지 않고 Preset만 적용한다. 일회성 또는 복합 조건 탐색은 Search session에서 수행한다. Preset은 Commit module에만 적용되며 Search result에는 암묵적으로 적용되지 않는다.

Rule은 다음 field를 대상으로 한다.

- Branch
- Author
- Message
- Changed file
- Date

Action은 `Hide`, `Show`다. 여러 Show rule은 모두 만족해야 하고, Hide rule은 하나라도 만족하면 제외한다. `My Jobs`, `3 Days` 같은 Preset은 Settings에서 편집하며 `$me`와 `last:3d` 같은 값을 사용할 수 있다. Commit toolbar에는 `PRESET` label 없이 최대 세 개의 Preset button만 표시한다. Settings는 세 개가 등록되면 Add preset을 비활성화하고, 저장값을 읽을 때도 유효한 Preset의 처음 세 개만 유지한다. 이전 settings에 남아 있는 `Highlight` rule은 load할 때 제외하며, 그 결과 유효한 rule이 하나도 없는 Preset도 표시하지 않는다.

## Edit Mode 진입

`Edit Mode`는 아래 조건을 만족하면 활성화된다.

1. Repository가 열려 있다.
2. Project/worktree 전환 중이 아니다.

선택한 commit은 entry 조건이 아니다. `Edit Mode`를 누르는 시점의 branch scope, `All branches`, Preset 결과를 포함한 **현재 table의 visible commit 순서**를 local draft로 복제한다. 따라서 mode 진입 전후에 history 범위, 선택한 worktree, scope, scroll 위치를 새로 정하거나 바꾸지 않는다.

Branch scope는 그 scope를 rewrite target으로 사용한다. `All branches`는 보던 graph/table을 바꾸지 않되, repository의 **default branch**를 rewrite target으로 사용한다. 이 mode는 default branch가 현재 attached local worktree에 checkout된 상태에서만 시작할 수 있다. GitGit은 server의 editable first-parent head range를 별도로 읽어 table 안의 target row만 식별하며, side branch row는 계속 선택·조회할 수 있지만 reorder와 metadata edit는 할 수 없다. default branch row 사이의 reorder만 executable draft로 projection되고, side row가 그 사이에 끼어 있는 visual 배치는 Apply payload에 포함하지 않는다.

실제 rewrite 대상의 first-parent 범위는 Review 단계에서 server가 다시 검증한다. active Preset 또는 server의 first-parent `HEAD` range와 정확히 맞지 않는 visual draft는 Review 결과에 이유를 표시하고 Apply를 열지 않는다. 이 경우에도 Edit Mode의 local draft는 유지되며, history refresh 또는 unfiltered target branch에서 새 draft를 시작할 수 있다. Remote-only default branch, root commit, 또는 default branch `HEAD`가 merge commit인 경우에는 `All branches` Edit Mode를 시작하지 않는다.

Detached `HEAD` worktree도 visual draft의 진입을 막지 않는다. 다만 Review는 target branch가 checkout된 worktree를 요구하므로, detached draft는 Apply 전에 해당 branch worktree에서 다시 열어야 한다.

Edit Mode는 modal이나 별도 workbench를 열지 않는다. 기존 `HistoryToolbar → CommitTable → Inspector` 구성을 그대로 유지한다. Inspector action row의 label은 진입하면 `Exit Edit`으로 바뀌고, Worktree/Branch selector와 Edit Mode rail, Commit table, pane resizer, Inspector를 하나로 감싸는 연속된 red boundary가 생긴다. Red는 mode 자체를 알리는 boundary와 focus outline에만 쓴다. 선택된 row는 일반 mode와 같은 blue selection을 유지해, 사용자가 직접 옮긴 row의 주황/노랑 강조와 구분되게 한다. Worktree/branch selector와 top-level repository action은 mode 동안 잠긴다.

## Edit Mode, Review, Apply

Edit Mode에서는 **Commit 순서와 message, Author, Author date draft를 local state에서 조정**한다. 모드 진입 직후 기존 History toolbar와 Commit table 사이에 compact Edit Mode rail이 나타난다. 이 rail은 `Review`를 소유하며, Review가 완료되면 같은 영역이 아래로 확장되어 Commit table을 밀어내는 review sheet가 된다. Inspector는 계속 선택한 한 commit의 상세 편집만 담당하며, 별도 Apply footer는 두지 않는다.

- Commit table은 평상시와 같은 compact row, ref badge, graph, 날짜 구분선, Inspector 선택 흐름을 유지한다.
- Draft는 Commit Page에서 보던 newest-first UI 순서 그대로 보관하고 표시한다.
- Branch scope에서는 각 commit row 전체를 click-and-drag하여 다른 row의 위 또는 아래에 drop할 수 있다. `All branches`에서는 default branch target row만 drag/drop 대상이며, side branch row는 read-only다. 별도 drag handle, 순번, 이동 안내 행은 추가하지 않는다.
- Drag 중 pointer가 이동 방향의 target row 안으로 30% 들어오면 주변 row가 위 또는 아래로 짧게 이동해 draft 위치를 미리 보여 준다. Preview reflow가 pointer 아래의 row를 바꿔도 작은 pointer 이동 안에서는 기존 preview를 유지해 왕복 animation을 막는다. Drop은 그 preview를 확정할 뿐이며, Graph는 animation 동안 잠시 숨겼다가 새 geometry와 함께 다시 표시한다.
- 사용자가 직접 옮긴 commit row만 주황/노랑 background와 left accent로 표시한다. 다른 행이 밀려 위치가 달라진 것만으로는 표시하지 않으며, 원래 위치로 돌아오면 강조가 사라진다. 옮긴 row가 동시에 선택된 경우에는 주황 background를 유지하고 left accent만 blue로 바꾼다.
- Row를 클릭하면 기존처럼 오른쪽 Inspector에서 해당 commit의 metadata, changed files와 diff를 읽는다. Edit Mode에서는 Inspector의 commit 제목, Author, Author date를 클릭해 그 자리에서 editor로 전환할 수 있다. Changed files와 diff는 현재도 read-only이며, 파일 내용 수정·삭제·복원 UI는 아직 제공하지 않는다. message textarea는 줄바꿈과 wrapping에 맞춰 최대 180px까지 자동으로 높이를 늘리며, 그 이상은 내부 scroll을 사용한다. Author date는 timezone을 보존하는 ISO 8601 문자열로 입력한다. Review 전에는 원문과 직접 달라진 commit hash 옆에, Review 후에는 replacement가 생길 verified range 전체의 hash 옆에 `→ will be changed`를 표시한다.
- Inspector 상단의 worktree action도 mode 동안은 실행하지 못한다.
- `Exit Edit Mode`를 누르면 아직 적용하지 않은 local reorder draft를 버리고, 보던 Commit Page의 history로 돌아간다.

`Review`는 가장 오래된 direct change를 anchor로 삼아 target branch의 first-parent `base..HEAD` stack을 server에서 다시 읽는다. `All branches` draft는 default branch row만 남긴 projection으로 검증한다. GitGit은 target branch를 자동 checkout하지 않는다. 검증 뒤 다음 영향만 요약한다.

- target branch와 `base → HEAD` lease
- 새 hash를 받는 commit 수
- 직접 수정한 commit 수와 뒤따라 replacement가 되는 commit 수
- local rewrite만 수행하며 push는 별도라는 점

Review가 완료된 뒤 draft를 한 글자라도 수정하거나 다시 drag하면 Review와 approval은 즉시 무효가 된다. Review sheet는 `Verified range: base → current HEAD`, 직접 수정한 commit의 변경 종류, replacement hash를 받는 전체 commit 수와 dependent replacement 수를 보여 준다. 전체 rewrite stack을 commit별 hash로 나열하지 않는다. Review가 유효할 때만 acknowledgement checkbox가 보이며, default branch는 default-branch history rewrite임을 명시한다. acknowledgement 이후 `Apply N`은 temporary worktree에서 replay하고, original `HEAD` backup ref를 만든 뒤 lease로 branch를 옮긴다. Author와 Author date override도 이 payload에 포함된다. Apply 중에는 draft editing과 Exit Edit Mode를 잠그며, 성공하면 history를 refresh하고 Edit Mode를 종료한다. 실패하면 local draft를 유지하고 review sheet에 오류를 남긴다.

현재 table edit flow는 file 수정·삭제·복원과 provenance checkbox를 아직 노출하지 않는다. 향후 optional provenance를 적용할 때 replacement commit에는 `rewritten from: <source hash>`를 기록하며, 과거 `GitGit-Rewritten-From:` trailer는 재작성 시 새 형식으로 교체한다.

## 현재 제공하지 않는 것

- Branch selector를 통한 checkout
- Merge commit reorder
- Commit split/squash/fixup 전용 workflow
- default branch 이외의 All branches target 선택 UI
- File 수정·삭제·복원 UI와 provenance opt-in UI
- Remote push와 force push
- Backup ref 정리 UI
