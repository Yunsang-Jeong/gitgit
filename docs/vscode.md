---
title: Visual Studio Code Extension
description: GitGit VS Code extension v0.1의 기능, helper protocol, 지원 platform과 배포 경계
audience:
  - human
  - ai-agent
status: active
document_type: specification
scope: vscode-extension
last_updated: 2026-08-14
---

# Visual Studio Code Extension

## 목적

GitGit VS Code extension은 현재 editor와 repository의 Git 맥락을 IDE 안에서 read-only로 탐색하는 surface다. Desktop UI를 Webview에 복제하지 않고, VS Code가 editor decoration과 view lifecycle을 담당하며 bundled Go helper가 system Git read를 수행한다.

v0.1은 다음 흐름을 기본으로 한다.

- 현재 file의 line blame을 editor에서 확인한다.
- 현재 repository의 history를 GitGit Search 조건으로 검색한다.
- 선택한 commit이 현재 file에 만든 변경 range와 삭제 anchor를 editor에서 확인한다.
- 현재 file의 commit history를 열고 Search 또는 change highlight의 대상으로 전환한다.

Extension과 helper는 commit, working tree, ref 또는 repository를 변경하지 않는다. Commit rewrite, checkout, fetch, pull, push와 worktree lifecycle은 VS Code integration의 범위가 아니다.

## v0.1 지원 범위

Extension ID는 `gitgit.gitgit`, extension version은 `0.1.0`이다. 다음 platform-specific VSIX 세 개가 같은 ID, version과 protocol version을 사용한다.

| VSIX target | Bundled helper |
| --- | --- |
| `darwin-arm64` | `dist/bin/gitgit-vscode-helper` |
| `darwin-x64` | `dist/bin/gitgit-vscode-helper` |
| `win32-x64` | `dist/bin/gitgit-vscode-helper.exe` |

각 VSIX에는 target과 일치하는 helper를 정확히 하나만 포함한다. 다른 platform helper, source tree, test fixture와 development dependency를 release VSIX에 함께 넣지 않는다.

v0.1은 local VS Code desktop과 local filesystem repository만 지원한다. 다음 환경은 지원하지 않는다.

- Generic VSIX와 Web extension
- Linux와 Windows ARM64
- WSL, Remote SSH, Dev Containers와 GitHub Codespaces
- VS Code 외 Cursor, Windsurf, JetBrains IDE

Unsupported platform에서는 다른 architecture의 helper를 실행하거나 system `git` 구현으로 조용히 대체하지 않는다. Extension이 지원 범위를 설명하고 기능을 비활성화한다.

## 기능 계약

### Line blame

Line blame은 active local file의 repository-relative path와 필요한 line range를 helper에 전달한다. 표시는 1-based line을 기준으로 commit, author, author time과 summary를 연결한다.

- Untitled, virtual document, repository 밖의 file에는 요청하지 않는다.
- Unsaved buffer를 지원하는 요청은 현재 document contents를 명시적으로 전달한다. Helper는 file을 쓰지 않는다.
- Document, active editor 또는 repository가 바뀌면 이전 결과를 폐기한다. Terminal에서 checkout/rebase처럼 editor event 없이 HEAD를 바꾼 경우에는 `GitGit: Refresh Repository`로 repository revision을 다시 발견한다.
- 새 요청이 이전 요청을 대체하면 `$/cancelRequest`를 보내고 stale result를 표시하지 않는다.

Line blame decoration은 GitGit이 계산한 정보만 더하며 VS Code의 built-in SCM decoration을 대체하지 않는다.

### File history와 Search

File history는 active file을 변경한 commit을 현재 repository scope에서 읽는다. Search는 Desktop Search와 같은 system Git domain을 사용하며 message, diff, file pattern과 scope option을 helper의 `search.run` 요청으로 전달한다.

- Search 실행 단위는 active local repository다.
- 결과의 commit과 file path는 editor navigation, revision content와 commit change highlight로 연결할 수 있다.
- 진행 상태는 `$/progress` notification으로 전달하고 취소된 실행의 partial result를 최종 결과로 채택하지 않는다.
- Extension은 Search session을 Git repository에 기록하거나 commit을 변경하지 않는다.

### Commit change highlight

Commit change highlight는 선택한 commit의 file diff에서 추가·수정된 target range와 삭제된 line의 anchor를 표시한다. 이는 working tree의 unstaged/staged 변경 gutter와 별개의 commit-relative overlay다.

- Highlight 기준은 commit과 repository-relative file path를 함께 보존한다.
- Helper의 `revision.content`와 active editor text가 정확히 같을 때만 commit blob 좌표를 적용한다. 이후 commit, dirty buffer 또는 line-ending 차이로 text가 달라졌으면 잘못된 line을 표시하지 않고 decoration을 비운다.
- Active file, repository 또는 선택 commit이 달라지면 이전 decoration을 제거한다.
- 삭제는 존재하지 않는 target line을 만들지 않고 인접 line anchor로 표시한다.
- Binary file, repository 밖의 path와 helper가 안전하게 해석할 수 없는 diff는 highlight하지 않는다.

Working tree에 더 이상 없는 deleted file에는 열 수 있는 active editor가 없으므로 v0.1은 deletion anchor를 표시하지 않는다. 선택한 historical commit과 현재 file content가 다른 경우의 정확한 overlay와 deleted-file snapshot은 revision virtual document를 도입할 때 확장한다.

## Architecture와 protocol

| Layer | 책임 |
| --- | --- |
| `apps/vscode/src/editor/` | Line blame, file history와 commit highlight state/decoration |
| `apps/vscode/src/search/` | Search expression, session state와 Webview |
| `apps/vscode/src/helper/` | Bundled helper process lifecycle와 protocol validation |
| `apps/vscode/src/shared/` | Repository identifier와 relative-path validation |
| `cmd/gitgit-vscode-helper/` | NDJSON JSON-RPC server entrypoint와 protocol adapter |
| `internal/app/`, `internal/gitexec/` | Desktop과 공유하는 Git read domain과 argument-safe system Git execution |
| `testdata/vscode-contract/protocol-v1.json` | Protocol v1의 canonical transport, method, path와 output limit contract |

Extension은 bundled helper를 stdio child process로 실행한다. Transport는 newline-delimited JSON-RPC 2.0이며 network listener를 열지 않는다. Protocol v1은 read-only이고 output limit은 8 MiB다.

Canonical method allowlist는 다음과 같다.

- `initialize`
- `repository.discover`
- `refs.list`
- `blame.lines`
- `history.file`
- `search.run`
- `revision.content`
- `diff.file`

Notification은 `$/cancelRequest`와 `$/progress`만 사용한다. Repository path는 존재하는 absolute worktree여야 하고 file path는 repository-relative이며 repository 밖으로 escape할 수 없다. Extension은 raw shell command나 임의 Git argv를 protocol에 전달하지 않는다.

Extension은 시작할 때 `initialize`로 protocol version과 capability를 협상한다. Extension version과 helper server version의 exact equality는 요구하지 않으며 protocol v1과 필요한 method가 호환되는지를 판정한다. Protocol mismatch, missing helper, executable failure와 malformed response는 명시적 error로 처리하고 다른 Git execution path로 fallback하지 않는다.

## 설치와 update 경계

v0.1 VSIX는 helper를 자체 포함하므로 GitGit Desktop 설치나 실행을 요구하지 않는다. 반대로 Desktop도 VSIX를 자동 설치, 교체 또는 제거하지 않는다.

개발·검증용 VSIX는 VS Code의 `Install from VSIX...` 또는 다음 CLI로 명시적으로 설치한다.

```sh
code --install-extension /absolute/path/to/gitgit-0.1.0-<target>.vsix
```

Side-loaded VSIX의 update 정책과 Marketplace 배포는 별도 release concern이다. Extension이 Desktop installer나 다른 executable을 download해 실행하는 흐름은 v0.1에 포함하지 않는다.

## 개발과 package

Repository root에서 protocol, Go core/helper와 extension을 함께 검증한다.

```sh
task vscode:check
```

세 target의 helper를 `CGO_ENABLED=0`으로 cross-build하고 VSIX를 package·audit한다.

```sh
task vscode:package
```

한 target만 확인하려면 allowlist에 있는 target을 명시한다.

```sh
task vscode:package:target TARGET=win32-x64
```

Artifact는 `dist/vscode/gitgit-0.1.0-<target>.vsix`에 생성되며 Git에는 포함하지 않는다. Package task는 `npm ci`, extension test·typecheck·build, root Go test를 먼저 통과한 뒤 helper를 만들고 `vsce package --target`을 실행한다. 마지막 content audit가 VSIX manifest의 target, extension ID/version, helper 수와 GOOS/GOARCH/CGO 정보를 확인하며 `src/`, `test/`, `tests/`, `testdata/`, `node_modules/`, source map과 protocol fixture가 package에 들어가면 실패한다.

같은 directory의 `release-manifest.json`은 다음 schema를 canonical key와 target 순서로 기록한다.

```json
{
  "schemaVersion": 1,
  "extensionId": "gitgit.gitgit",
  "extensionVersion": "0.1.0",
  "helperServerVersion": "0.1.0",
  "protocolVersion": 1,
  "artifacts": [
    {
      "target": "darwin-arm64",
      "file": "gitgit-0.1.0-darwin-arm64.vsix",
      "sha256": "<64 lowercase hex characters>"
    }
  ]
}
```

`helperServerVersion`과 `protocolVersion`은 package 시점에 host helper의 실제 `initialize` 응답에서 읽는다. Package script는 이 응답을 canonical protocol fixture와 비교하고, manifest를 다시 읽어 package.json version, helper metadata, target 목록과 각 VSIX의 재계산한 SHA-256이 모두 일치하는지 audit한다. `task vscode:package`는 세 target을 고정 순서로 기록하고, `task vscode:package:target TARGET=<target>`은 기존 directory에 다른 VSIX가 남아 있어도 이번에 생성한 target 하나만 기록한다.

GitHub Actions의 `.github/workflows/vscode.yml`도 Ubuntu package job에서 같은 full gate를 실행하고 audit를 통과한 세 VSIX와 `release-manifest.json`만 workflow artifact로 보존한다. 이어지는 `windows-latest` job은 POSIX 전용 fixture가 있는 다른 root package까지 과장해 실행하지 않고, `internal/vscodehelper`의 real-repository test를 Windows에서 실행한다. 그 다음 manifest의 `win32-x64` SHA-256을 다시 확인하고 해당 VSIX에서 helper를 추출해 native `initialize`와 `repository.discover`를 호출한다. 이 smoke는 packaged helper의 server/protocol version이 manifest와 같은지, read-only/offline capability, system Git discovery와 checkout 불변성을 확인한다.

Windows helper smoke도 VS Code Extension Host를 실행하지는 않는다. 각 target VSIX를 대응 OS/architecture의 clean VS Code profile에 설치해 activation과 실제 editor flow를 확인하는 절차는 별도 release gate로 유지한다.

## Release invariant

한 release의 세 VSIX는 다음을 만족해야 한다.

1. Extension ID와 version이 모두 같다.
2. Protocol fixture와 helper protocol version이 같다.
3. Target별 helper가 정확히 하나만 들어 있고 executable name이 target과 일치한다.
4. Extension bundle과 runtime asset 외 source, test와 다른 platform binary가 들어 있지 않다.
5. Go core/helper test, extension test·typecheck·build와 packaged VSIX content audit를 통과한다.
6. Release manifest에 extension version, helper server version, protocol version, target과 artifact digest를 각각 기록한다.

Compile 또는 package 성공만으로 release 완료를 판정하지 않는다. 각 target VSIX를 대응 OS/architecture의 clean VS Code profile에 설치해 activation, helper `initialize`, repository discovery와 세 사용자 flow를 실행해야 한다.

## v0.1 이후

다음 항목은 protocol과 platform 배포가 안정된 뒤 별도 범위로 다룬다.

- File history graph 전용 Webview와 richer graph interaction
- Historical revision virtual document와 서로 다른 revision 사이의 line mapping
- Marketplace 자동 publish와 Desktop↔VS Code install handoff
- Windows Desktop installer와 extension discovery 통합
- Linux, Windows ARM64와 remote extension host
- Write operation과 AI 기능
