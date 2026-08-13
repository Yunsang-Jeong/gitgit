# GitGit for Visual Studio Code

GitGit brings read-only Git investigation into a local VS Code workspace. Version 0.1 bundles a platform-specific Go helper and supports line blame, repository Search, file history, and commit-relative change highlights without requiring GitGit Desktop.

## Supported platforms

- macOS Apple Silicon (`darwin-arm64`)
- macOS Intel (`darwin-x64`)
- Windows x64 (`win32-x64`)

Generic/Web extensions, Linux, Windows ARM64, WSL, Remote SSH, Dev Containers, and GitHub Codespaces are not supported in version 0.1.

The extension and bundled helper are read-only and offline. They do not fetch, checkout, rewrite, push, or otherwise mutate a repository.

See the [canonical VS Code integration specification](https://github.com/Yunsang-Jeong/gitgit/blob/main/docs/vscode.md) for the protocol, security, and release boundaries.
