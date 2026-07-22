# AGENTS.md

이 파일은 상세 설계를 담지 않는다. AI Agent가 작업에 필요한 canonical document를 빠르게 찾고, 문서 변경 범위를 판단하기 위한 index만 유지한다.

## 문서 index

| 문서 | Canonical scope | 변경 시점 |
| --- | --- | --- |
| [docs/development.md](docs/development.md) | **Required gate**: `wails dev` browser-first 개발·검증 절차 | Product code 또는 test를 변경하는 모든 작업의 시작과 완료 전 |
| [README.md](README.md) | 사람용 프로젝트 진입점, 빠른 시작, 문서 연결 | 첫 사용 경험이나 최상위 제품 설명이 바뀔 때 |
| [docs/overview.md](docs/overview.md) | 전체 개요, 방향성, architecture, 구현 현황과 공통 경계 | 제품 구조, 지원 범위, persistence 또는 build 흐름이 바뀔 때 |
| [docs/commit.md](docs/commit.md) | Commit 화면과 commit rewrite 규칙 | history, Preset, Inspector, editing 조건이 바뀔 때 |
| [docs/worktree.md](docs/worktree.md) | Worktree 화면과 lifecycle 규칙 | worktree 표시 상태, 선택, action, 제거 조건이 바뀔 때 |
| [docs/search.md](docs/search.md) | Search session과 query semantics | 검색 조건, scope, session lifecycle, 결과 모델이 바뀔 때 |
| [docs/remote-branches.md](docs/remote-branches.md) | Remote branch 탐색과 Settings 관리 계획 | Remote branch 조회, 선택, 포함 정책을 구현하거나 계획을 바꿀 때 |

## 문서 유지 규칙

1. 상세 동작은 해당 `docs/*.md`에 기록한다. `AGENTS.md`에는 요약을 복제하지 않는다.
2. 사용자 진입에 필요한 최소 정보만 `README.md`에 둔다.
3. `docs/**/*.md`는 아래 frontmatter key를 같은 순서로 사용한다. Entry point인 `README.md`와 `AGENTS.md`에는 frontmatter를 두지 않는다.
   - `title`
   - `description`
   - `audience`
   - `status`
   - `document_type`
   - `scope`
   - `last_updated`
4. 기능 변경은 의미적으로 가장 가까운 한 문서를 우선 수정한다. 여러 module에 영향을 줄 때만 `overview.md`도 갱신한다.
5. 구현과 문서가 다르면 실제 code와 test를 근거로 동작을 확인한 뒤 문서를 갱신한다.
6. 새 문서를 추가하거나 경로를 바꾸면 이 index와 연결된 상대 link를 함께 갱신한다.

작업 완료 전에는 변경한 문서의 link, frontmatter, `git diff --check`와 관련 test를 확인한다.
