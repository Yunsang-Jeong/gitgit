# GitGit

개인적인 Git 이용 패턴과 취향이 뒤섞인 프로젝트이다.

- **Commit**: worktree, branch를 선택하고 commit을 열람하고 수정할 수 있다.
- **Worktree**: 연결된 worktree의 branch, path, dirty/locked/sparse/merge 상태를 비교하고 정리한다.
- **Search**: 정밀한 blame을 위해 조금은 변태같이 commit을 뒤적거린다.

## QuickStart

요구 환경:

- Apple Silicon Mac, macOS 11 이상
- Go 1.26.x
- Node.js 22.12 이상과 npm
- `worktree --porcelain -z`, `sparse-checkout check-rules`를 지원하는 Git

```sh
$ make install
```

## Docs

모든 스펙과 방향성은 docs에서 확인한다.

| 문서 | 내용 |
| --- | --- |
| [Development Gate](docs/development.md) | `wails dev` browser-first 개발·검증 절차와 통과 기준 |
| [Overview](docs/overview.md) | 프로젝트 개요, 방향성, architecture, 현재 구현 상태 |
| [Commit](docs/commit.md) | Commit 화면, history 범위, Filter, Inspector, commit editing |
| [Worktree](docs/worktree.md) | worktree 모델, 표시 상태, 선택과 제거 규칙 |
| [Search](docs/search.md) | Search session, AND/OR query, scope, 결과와 비용 모델 |


## Develop

## testdata (subgit)

100개 commit, 12개 branch, 12개 worktree와 dirty/locked/sparse/detached 상태를 가진 `subgit/` fixture를 생성할 수 있다.

```sh
make subgit
make subgit-reset
```

`make subgit`은 GitGit marker가 없는 기존 directory를 덮어쓰지 않는다. `make subgit-reset`은 관리되는 fixture만 재생성한다.

## test

```sh
make dev-browser
# Codex sidebar browser에서 http://localhost:34116 열기

make check
```

Product code나 test를 변경할 때는 Wails browser bridge에서 대상 flow를 먼저 확인하고, 자동화 검증 뒤 같은 flow를 다시 확인한다. 상세 기준과 예외는 [Development Gate](docs/development.md)를 따른다. `make dev`는 `make dev-browser`의 alias다.
