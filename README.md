# GitGit

개인적인 Git 이용 패턴과 취향이 뒤섞인 프로젝트이다.

- **Commit**: worktree, branch를 선택하고 commit을 열람하며, 제한된 local history의 순서와 metadata를 수정할 수 있다. Changed files와 diff는 현재 read-only다.
- **Worktree**: 연결된 worktree의 branch, path, dirty/locked/sparse/merge 상태를 비교하고 정리한다.
- **Search**: 정밀한 blame을 위해 조금은 변태같이 commit을 뒤적거린다.
- **VS Code**: bundled read-only helper로 line blame, Search와 commit change highlight를 editor 안에서 제공한다.

## QuickStart

요구 환경:

- macOS 11 이상을 실행하는 Mac
- Go 1.26.x
- Node.js 22.12 이상과 npm
- go-task 3.50 이상 (`task`)
- `worktree --porcelain -z`, `sparse-checkout check-rules`를 지원하는 Git

```sh
$ task install
```

실행할 GUI app은 `$HOME/Applications/GitGit.app` 하나다. `task install`은 이 app을 교체한 뒤 중간 build bundle을 삭제한다.

## Docs

모든 스펙과 방향성은 docs에서 확인한다.

| 문서 | 내용 |
| --- | --- |
| [Development Gate](docs/development.md) | `wails dev` browser-first 개발·검증 절차와 통과 기준 |
| [Overview](docs/overview.md) | 프로젝트 개요, 방향성, architecture, 현재 구현 상태 |
| [Commit](docs/commit.md) | Commit 화면, history 범위, Preset, Inspector, commit editing |
| [Worktree](docs/worktree.md) | worktree 모델, 표시 상태, 선택과 제거 규칙 |
| [Search](docs/search.md) | Search session, AND/OR query, scope, 결과와 비용 모델 |
| [Remote branches](docs/remote-branches.md) | Local remote-tracking ref를 read-only로 탐색하는 기능과 경계 |
| [Visual Studio Code](docs/vscode.md) | VS Code extension v0.1 기능, helper protocol, 지원 target과 배포 경계 |


## Develop

## testdata (subgit)

100개 commit, 12개 branch, 12개 worktree와 dirty/locked/sparse/detached 상태를 가진 `subgit/` fixture를 생성할 수 있다.

```sh
task fixture:create
task fixture:reset
```

`task fixture:create`은 GitGit marker가 없는 기존 directory를 덮어쓰지 않는다. `task fixture:reset`은 관리되는 fixture만 재생성한다. 기존에 익숙한 `task subgit`, `task subgit-reset`도 각각 alias로 제공한다.

`subgit/`은 app을 실제로 조작해 보는 개발용 playground다. Edit Mode로 history를 rewrite하면 내용이 canonical fixture와 달라지므로, `task fixture:create`은 HEAD commit 수뿐 아니라 모든 ref에서 도달 가능한 commit 수까지 확인해 이 상태를 감지하고 `task fixture:reset`을 안내한다. Go integration test는 `subgit/`을 사용하지 않고 `~/Library/Caches/GitGit/test-fixtures/` 아래에 별도 fixture를 만들어 쓰므로, playground를 마음대로 바꿔도 test 결과에 영향을 주지 않는다.

## test

```sh
task dev:browser
# Codex sidebar browser에서 http://localhost:34116 열기

task check
```

Product code나 test를 변경할 때는 Wails browser bridge에서 대상 flow를 먼저 확인하고, 자동화 검증 뒤 같은 flow를 다시 확인한다. 상세 기준과 예외는 [Development Gate](docs/development.md)를 따른다. `task dev`와 `task dev-browser`는 `task dev:browser`의 alias다.

Taskfile의 task는 macOS에서만 실행된다. Build는 현재 Mac의 architecture를 따르며, GUI 환경에서 npm 경로를 찾지 못하면 `task build NPM=/absolute/path/to/npm`처럼 지정할 수 있다.

Taskfile은 frontend dependency와 build input에는 Task의 `sources`/`generates` checksum cache를 사용하지만, release metadata와 signing이 필요한 `task bundle`은 매번 새로 수행한다. `task build`, `task check`, `task install`, `task dev:browser`는 종료할 때 `apps/desktop/build/bin`의 임시 app bundle을 정리한다. 실제 local bundle을 보존해야 할 때만 `task bundle`을 사용하며, 결과는 `apps/desktop/build/bin/GitGit.app`에 남는다.
