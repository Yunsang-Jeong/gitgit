---
title: Search Module
description: Search session lifecycle, AND/OR query semantics, revision scope와 결과 모델
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: search
last_updated: 2026-09-07
---

# Search Module

## 목적

Search는 commit history 전체를 scan할 수 있는 비교적 비용이 큰 작업이다. GitGit은 이를 Commit toolbar의 일회성 drawer로 처리하지 않고, 독립 workspace와 session으로 분리한다.

각 session은 검색 대상과 query, 마지막 성공 결과를 보존한다. 다른 화면이나 session으로 이동해도 자동으로 같은 검색을 다시 실행하지 않는다.

## Search session

Search 화면에 들어가는 것만으로 session을 만들지 않는다. 처음에는 empty state를 표시하며 좌측 sidebar 또는 본문의 `+` 버튼을 눌렀을 때만 session을 만든다. Sidebar 제목은 Commit 화면의 history와 혼동되지 않도록 `Sessions`를 사용한다. Session alias는 sidebar에서 바로 수정할 수 있고 `×`로 삭제한다. Sidebar는 project, query, status와 실행 시각을 한눈에 비교할 수 있도록 유지하되 결과 조사 공간을 침범하지 않도록 compact width와 72px session row를 사용한다. Inspector 공간이 더 필요하면 sidebar를 40px reopen rail로 접을 수 있다.

현재 status는 다음 중 하나다. 검색 전 session에는 불필요한 `draft` badge를 붙이지 않는다.

| Status | 의미 |
| --- | --- |
| `running` | 현재 backend scan을 실행 중 |
| `ready` | 현재 query와 마지막 성공 결과가 일치 |
| `stale` | 결과를 만든 뒤 query 또는 scope를 수정함 |
| `error` | 마지막 실행이 실패함 |

Session이 유지하는 값:

- Project root
- Worktree root
- Branch 또는 All refs scope
- Message/DIFF/FILE conditions와 AND/OR operator
- Glob/Regex engine
- Author, Since, Until
- 마지막 성공 결과, scanned count와 selection
- 마지막 Search 실행 timestamp

Session은 frontend process memory에만 존재한다. Application을 종료하면 삭제되며 Recent search나 disk persistence는 제공하지 않는다.

Backend는 한 번에 하나의 active search만 실행한다. 다른 session으로 전환하거나 repository operation이 시작되면 진행 중 search를 cancel할 수 있다.

## 검색 대상

상단 control은 다음 순서를 사용한다.

```text
[Project]  [Worktree]  [Branch]  [Search]
```

Session sidebar header와 target toolbar는 36px section bar를 사용한다. Project, Worktree, Branch, Search와 composer toggle은 이 bar 안에서 26px compact control로 정렬한다.

- **Project**: Registered project 또는 새 repository를 선택한다.
- **Worktree**: 검색이 실행될 실제 repository root를 선택한다.
- **Branch**: checkout 없이 revision scope만 선택한다.
- **All branches**: UI의 branch scope 이름이다. Backend search request에서는 `All refs`로 실행된다.

Revision scope는 toolbar의 Branch selector 하나만 제어한다. 같은 값을 다른 이름(`All branches` / `All refs`)으로 두 번 노출하지 않도록 composer의 filter row에는 별도 Scope 입력을 두지 않는다.

Project나 Worktree를 변경하면 기존 결과는 버리고 query만 새 대상에 맞게 유지한다. 새 대상의 checked-out branch를 초기 scope로 사용한다.

## Query condition

Condition source는 세 가지다.

- **Message**: 전체 commit message를 대상으로 match한다.
- **DIFF**: Unified diff의 context나 metadata가 아니라 실제 added/deleted line content를 대상으로 match한다.
- **FILE**: Commit에서 변경된 path를 대상으로 match한다. Rename된 file은 old/new lineage를 추적한다.

Query composer는 condition row를 여러 개 쌓는 visual builder가 아니라 하나의 Expression input을 사용한다. 각 condition은 `MSG:`, `DIFF:`, `FILE:` source prefix로 시작하고 `AND` 또는 `OR`로 연결한다. 괄호를 직접 입력해 우선순위를 표현하며 한 condition 앞뒤에는 각각 최대 8개 group boundary를 둘 수 있다. 공백, `AND`/`OR` 또는 괄호를 literal value로 검색해야 하면 single/double quote로 value를 감싼다.

Expression 아래 helper는 빈 입력에서 source와 operator 사용법을 안내하고, 입력 중에는 누락된 colon, value, 다음 source와 닫는 괄호 같은 구문 오류를 즉시 표시한다. Valid expression에는 condition 수, Glob/Regex hint, Enter 실행 방법 또는 현재 결과와의 stale/applied 상태를 표시한다. 별도의 condition chip summary는 중복 표시하지 않는다. Engine, Author, Since, Until은 동일 너비 grid로 정렬하며 sidebar와 main panel은 같은 outer gutter를 사용한다. Search action 우측의 접기/펼치기 chevron으로 composer 전체를 접어 결과와 Inspector에 세로 공간을 돌려줄 수 있고, 접어도 작성 중인 expression과 scope는 유지한다.

괄호가 없으면 AND가 OR보다 먼저 평가되며, group이 있으면 해당 범위가 우선한다. Frontend와 backend는 생성된 expression을 모두 검증한다.

예를 들어 다음 query는 아래처럼 해석된다.

```text
Message:*cache* OR FILE:**/*.go AND DIFF:*context*

Message:*cache* OR (FILE:**/*.go AND DIFF:*context*)

(Message:*cache* OR FILE:**/*.go) AND DIFF:*context*
```

Scope, Author, Since, Until은 개별 condition이 아니라 query 전체에 적용된다.

## Matching engine

### Glob

기본 engine이다.

- File path에서 `*`는 하나의 path segment 안에서 match한다.
- `**`는 directory separator를 넘어 match한다.
- `**/*.go`는 `main.go`와 `internal/app/search.go`를 모두 포함한다.
- Message와 DIFF text의 `*`는 `/`와 line boundary도 넘을 수 있다.
- 부분 text를 찾으려면 일반적으로 `*fix*`처럼 양쪽 wildcard가 필요하다.

Commit context menu에서 Message를 새 Search session으로 보내면 GitGit이 glob special character를 escape하고 양쪽에 `*`를 붙인다.

### Regex

Go regular-expression syntax를 사용한다. `^`, `$`로 anchor하지 않으면 substring match로 동작한다.

## Revision과 date scope

기본 scope는 현재 `HEAD`다. 다음 값도 사용할 수 있다.

- All refs
- Local branch 또는 tag
- `v1.2.0..HEAD` 같은 revision range
- `main...feature/auth` 같은 symmetric difference
- `abc123^!` 같은 exact commit expression

All refs와 별도 revision expression은 동시에 사용할 수 없다. Revision value가 Git option처럼 `-`로 시작하면 거부한다.

Author는 Git author filter로 전달한다. Since/Until은 absolute date/time과 `last:3d`, `last:30d` 같은 relative input을 지원하며 실행 전에 ISO timestamp로 정규화한다. Text input 옆의 calendar control은 native date/time picker를 열고 선택한 local wall time을 같은 display timestamp 형식으로 입력한다. 따라서 picker를 사용해도 relative input을 직접 입력하는 기존 경로는 유지된다.

## 실행과 결과

`Search`를 눌러야 backend scan이 시작된다. Query 편집만으로 자동 재실행하지 않는다.

Valid한 Query input은 즉시 session에 반영되지만 기존 result는 자동으로 바꾸지 않는다. 입력 중인 invalid expression은 helper에서 바로 설명하고 Search를 비활성화하며 마지막 valid query를 손상시키지 않는다. 마지막 성공 결과와 현재 query가 달라지면 helper에 `Expression changed` 상태를 표시한다. 이전의 `Draft` 표현은 사용하지 않는다.

실행 중에는 실제 방문한 commit 수와 scan 대상 commit 수를 progress로 표시하고 Cancel action을 제공한다. Progress event는 repository 크기와 비례해 무제한 발생하지 않도록 한 실행당 약 200회 이하의 간격으로 제한한다. 실패하거나 새 query가 실행 중이어도 이전 성공 결과가 있으면 교체하지 않고 유지한다.

Backend는 scope의 commit metadata를 한 번의 `git log`로 읽으며 별도 `rev-list --count`나 result별 metadata 조회를 실행하지 않는다. Changed-file metadata는 500-commit 단위 `git diff-tree --stdin` batch로 읽고, result limit 뒤 실제 matching commit 하나를 확인해 `has_more`를 결정하면 이후 batch를 실행하지 않는다. Message/FILE-only search는 table에 쓰지 않는 unified diff payload를 만들지 않으며, DIFF condition의 정확한 file별 content 판정에만 필요한 file diff를 지연해서 읽는다.

새 Search가 시작되거나 Cancel을 누르면 이전 search context와 system Git process를 함께 취소한다. Partial result는 event로 흘려보내지 않고 완료된 response만 session의 마지막 성공 결과로 교체한다. 이 경계는 빠른 중간 표시보다 session 결과의 원자성과 retry 동작을 우선한다.

Backend는 Boolean expression을 file 단위로 평가한 뒤 같은 commit의 hit를 하나의 result로 합친다. 따라서 서로 다른 두 file의 `FILE:`와 `DIFF:` hit가 하나의 `AND`를 잘못 만족하지 않으면서도, UI와 wire의 결과 단위는 처음부터 commit이다. Search의 기본 사용자 흐름은 다음과 같다.

```text
조건 작성 → Search 실행 → matching commit 선택 → Inspector에서 changed files 확인 → file diff 확인
```

- Commit당 한 행
- 여러 match source는 한 행의 badge로 합침. Match column은 고정 폭이 아니라 badge 내용에 맞춰 넓어지므로 `Message`, `DIFF`, `FILE`이 모두 붙어도 잘리지 않음
- Result table은 topology graph와 File column을 표시하지 않음
- Result table은 별도 column header와 내부 grid separator 없이 Commit 화면과 같은 32px row 높이를 사용
- Branch/ref 정보는 Message와 같은 행의 작은 badge로 표시
- 선택한 commit은 우측 Inspector에서 metadata, 전체 changed files와 file diff를 확인
- FILE/DIFF가 실제로 일치한 file만 Inspector에서 `Match`로 표시하고 Message-only match는 모든 changed file을 match로 표시하지 않음
- Result contract는 `matched_files`와 commit 전체 `changed_files`를 분리하며, Message-only empty commit도 file을 꾸며내지 않고 결과에 포함함
- Status bar에는 scanned commit 수와 matching commit 수를 표시
- Result table은 Commit table과 같은 ARIA table 구조와 roving tabindex, 방향키 이동 규칙을 사용

현재 request limit은 unique matching commit 250개다. 한 commit의 많은 matching file이 limit을 소진하지 않는다. Backend가 limit 뒤 실제 matching commit을 하나 더 확인하면 `has_more`를 true로 반환하므로 client는 `250+`처럼 truncated 결과임을 표시하고 exhaustive count로 오해시키지 않는다.

Commit module의 Preset은 Search result에 적용되지 않는다. Search는 condition 자체가 결과 범위를 정의한다.

## 현재 제공하지 않는 것

- Search session disk persistence
- 여러 search의 동시 background queue
- Result pagination 또는 `Load more`
- Search result에서 직접 commit rewrite

## Persistence와 background queue 계획

Session persistence와 background queue는 아직 구현하지 않았다. 조사에서 확인한 제약과 결정해야 할 사항을 남긴다.

### 재사용할 persistence 계층

`internal/desktop/project_store.go`의 `ProjectStore`가 그대로 본이 된다. Version 봉투 JSON, temp file에 쓴 뒤 `chmod 0600`·`Sync`·`rename`으로 교체하는 원자적 write, file이 없으면 빈 값을 돌려주는 관대한 read, 그리고 `DesktopApp.startup`의 주입 seam으로 test에서 가짜를 넣을 수 있는 구조를 갖췄다. `$XDG_CONFIG_HOME/gitgit/`에 형제 file을 두면 된다.

`PersistentCache`는 쓰지 않는다. 스스로 disposable로 문서화돼 있고 ref fingerprint가 바뀌면 통째로 지워진다.

### 결정해야 할 것

**저장 범위.** Session이 보존하는 "마지막 성공 결과"는 최대 250 commit에 각 commit의 전체 `changed_files`가 붙어 JSON blob으로 크다. Query만 저장하고 결과는 재실행할지, 결과까지 저장하고 ref 변화에 따른 무효화 규칙을 둘지 정해야 한다.

**Repository 소유권.** Background queue의 존재 이유는 지금 보고 있지 않은 worktree를 대상으로 검색을 돌리는 것이다. 그런데 `Service`는 단일 `s.repository`를 들고 있고, `beginReservedRepositorySwitch`는 repository를 전환할 때 진행 중인 검색을 무조건 취소한다. Job마다 `gitexec.OpenRepository`로 handle을 열거나 job과 함께 repository를 넘기는 구조가 필요하다.

**동시성.** `Service`는 동시 검색을 하나만 지원한다. `searchCancel`과 `searchID` 한 쌍으로 새 검색이 이전 검색을 대체하는 구조이며, queue는 이 가정을 바꾼다.

**Progress 배선.** Wails event는 `search:progress` 하나뿐이고 `request_id`로만 구분한다. Queue에는 session/job ID와 terminal done/cancelled event가 필요하다. `internal/vscodehelper/search_diff.go`의 `progressReporter`가 bounded·thread-safe·terminal event를 갖춘 참고 구현이다.

### 선행 작업

Session lifecycle은 `App.svelte`에 인라인이라 test할 수 없다. `lib/search-session.ts`로 추출하는 것이 persistence test의 전제조건이다. `apps/vscode/src/search/model.ts`는 desktop에 없는 reducer 형태(`phase: 'idle' | 'running' | 'ready' | 'stale' | 'error'`와 `reduceSearchState`)를 이미 갖고 있어 참고 대상이다.

### 지켜야 할 성능 보장

`TestHistoryAndSearchBatchChangedFileMetadata`는 page당 `diff-tree --stdin` batch 한 번, 중복 count command 없음, message-only 검색에 eager diff 없음을 단언한다. Queue를 도입해도 계속 통과해야 한다. 큰 repository의 read 성능은 `task check:performance`가 별도로 지킨다.
