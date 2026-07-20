---
title: Search Module
description: Search session lifecycle, AND/OR query semantics, revision scope와 결과 모델
audience:
  - human
  - ai-agent
status: active
document_type: module
scope: search
last_updated: 2026-07-20
---

# Search Module

## 목적

Search는 commit history 전체를 scan할 수 있는 비교적 비용이 큰 작업이다. GitGit은 이를 Commit toolbar의 일회성 drawer로 처리하지 않고, 독립 workspace와 session으로 분리한다.

각 session은 검색 대상과 query, 마지막 성공 결과를 보존한다. 다른 화면이나 session으로 이동해도 자동으로 같은 검색을 다시 실행하지 않는다.

## Search session

좌측 sidebar의 `+` 버튼으로 session을 만든다. 현재 status는 다음 중 하나다.

| Status | 의미 |
| --- | --- |
| `draft` | 아직 검색하지 않은 query |
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

Session은 frontend process memory에만 존재한다. Application을 종료하면 삭제되며 Recent search나 disk persistence는 제공하지 않는다.

Backend는 한 번에 하나의 active search만 실행한다. 다른 session으로 전환하거나 repository operation이 시작되면 진행 중 search를 cancel할 수 있다.

## 검색 대상

상단 control은 다음 순서를 사용한다.

```text
[Project]  [Worktree]  [Branch]  [Search]
```

- **Project**: Registered project 또는 새 repository를 선택한다.
- **Worktree**: 검색이 실행될 실제 repository root를 선택한다.
- **Branch**: checkout 없이 revision scope만 선택한다.
- **All branches**: UI의 branch scope 이름이다. Backend search request에서는 `All refs`로 실행된다.

Project나 Worktree를 변경하면 기존 결과는 버리고 query만 새 대상에 맞게 유지한다. 새 대상의 checked-out branch를 초기 scope로 사용한다.

## Query condition

Condition source는 세 가지다.

- **Message**: 전체 commit message를 대상으로 match한다.
- **DIFF**: Unified diff의 context나 metadata가 아니라 실제 added/deleted line content를 대상으로 match한다.
- **FILE**: Commit에서 변경된 path를 대상으로 match한다. Rename된 file은 old/new lineage를 추적한다.

첫 condition 뒤의 각 condition에는 `AND` 또는 `OR`를 지정한다. Parenthesis UI는 아직 없으며 AND가 OR보다 먼저 평가된다.

예를 들어 다음 query는 아래처럼 해석된다.

```text
Message:*cache* OR FILE:**/*.go AND DIFF:*context*

Message:*cache* OR (FILE:**/*.go AND DIFF:*context*)
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

Author는 Git author filter로 전달한다. Since/Until은 absolute date/time과 `last:3d`, `last:30d` 같은 relative input을 지원하며 실행 전에 ISO timestamp로 정규화한다.

## 실행과 결과

`Search`를 눌러야 backend scan이 시작된다. Query 편집만으로 자동 재실행하지 않는다.

실행 중에는 scanned progress와 Cancel action을 표시한다. 실패하거나 새 query가 실행 중이어도 이전 성공 결과가 있으면 교체하지 않고 유지한다.

Backend match는 file 단위지만 UI는 같은 commit의 match를 한 행으로 합친다.

- Commit당 한 행
- 여러 match source는 한 행의 badge로 합침
- 여러 changed file은 count와 tooltip으로 표시
- Status bar에는 scanned commit 수와 matching commit 수를 표시

현재 request limit은 file-level match 250개다. 매우 많은 file이 match하는 commit은 이 limit을 빠르게 사용할 수 있으므로 결과가 repository 전체의 exhaustive count라고 가정해서는 안 된다.

Commit module의 Filter는 Search result에 적용되지 않는다. Search는 condition 자체가 결과 범위를 정의한다.

## 현재 제공하지 않는 것

- Search session disk persistence
- Saved search 이름 변경과 삭제
- 여러 search의 동시 background queue
- Parenthesis를 이용한 임의 boolean expression
- Result pagination 또는 `Load more`
- Search result용 Inspector pane
- Search result에서 직접 commit rewrite
