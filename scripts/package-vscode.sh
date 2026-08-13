#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
EXTENSION_DIR="${ROOT_DIR}/apps/vscode"
HELPER_PACKAGE="./cmd/gitgit-vscode-helper"
PROTOCOL_FIXTURE="${ROOT_DIR}/testdata/vscode-contract/protocol-v1.json"
OUTPUT_DIR="${VSCODE_OUTPUT_DIR:-${ROOT_DIR}/dist/vscode}"
GO_BIN="${GO:-go}"
NODE_BIN="${NODE:-node}"
NPM_BIN="${NPM:-npm}"

SUPPORTED_TARGETS="darwin-arm64 darwin-x64 win32-x64"
RUN_CHECKS=1
CHECK_ONLY=0
TARGETS=""
TEMP_DIR=""

log() {
  printf '[vscode-package] %s\n' "$*"
}

fail() {
  printf '[vscode-package] error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: scripts/package-vscode.sh [options]

Build and audit all v0.1 platform-specific VSIX artifacts by default.

Options:
  --target TARGET     Package one supported target. May be repeated.
  --output-dir PATH   Write VSIX artifacts under PATH.
  --check-only        Run protocol, Go, and npm checks without packaging.
  --skip-checks       Package an already checked and built extension.
  -h, --help          Show this help.

Supported targets: darwin-arm64, darwin-x64, win32-x64.
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

target_supported() {
  case "$1" in
    darwin-arm64 | darwin-x64 | win32-x64) return 0 ;;
    *) return 1 ;;
  esac
}

append_target() {
  target_supported "$1" || fail "unsupported VSIX target: $1"
  case " ${TARGETS} " in
    *" $1 "*) ;;
    *) TARGETS="${TARGETS}${TARGETS:+ }$1" ;;
  esac
}

cleanup() {
  rm -f -- \
    "${EXTENSION_DIR}/dist/bin/gitgit-vscode-helper" \
    "${EXTENSION_DIR}/dist/bin/gitgit-vscode-helper.exe"
  rmdir "${EXTENSION_DIR}/dist/bin" 2>/dev/null || true
  if [ -n "${TEMP_DIR}" ] && [ -d "${TEMP_DIR}" ]; then
    rm -rf -- "${TEMP_DIR}"
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      [ "$#" -ge 2 ] || fail "--target requires a value"
      append_target "$2"
      shift 2
      ;;
    --output-dir)
      [ "$#" -ge 2 ] || fail "--output-dir requires a value"
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --check-only)
      CHECK_ONLY=1
      shift
      ;;
    --skip-checks)
      RUN_CHECKS=0
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1"
      ;;
  esac
done

[ ! "${CHECK_ONLY}" -eq 1 ] || [ "${RUN_CHECKS}" -eq 1 ] || fail "--check-only and --skip-checks cannot be combined"
[ -n "${OUTPUT_DIR}" ] || fail "VSIX output directory cannot be empty"
[ -d "${EXTENSION_DIR}" ] || fail "VS Code extension directory not found: ${EXTENSION_DIR}"
[ -f "${EXTENSION_DIR}/package.json" ] || fail "VS Code package.json not found"
[ -f "${EXTENSION_DIR}/package-lock.json" ] || fail "VS Code package-lock.json not found"
[ -f "${PROTOCOL_FIXTURE}" ] || fail "canonical protocol fixture not found: ${PROTOCOL_FIXTURE}"
[ -f "${ROOT_DIR}/cmd/gitgit-vscode-helper/main.go" ] || fail "helper entrypoint not found: ${HELPER_PACKAGE}"

require_command "${GO_BIN}"
require_command "${NODE_BIN}"
require_command "${NPM_BIN}"
require_command unzip

trap cleanup EXIT INT TERM
TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gitgit-vscode-package.XXXXXX")"

validate_protocol_fixture() {
  "${NODE_BIN}" - "${PROTOCOL_FIXTURE}" <<'NODE'
const fs = require('node:fs')

const fixturePath = process.argv[2]
const fixture = JSON.parse(fs.readFileSync(fixturePath, 'utf8'))
const requiredMethods = [
  'initialize',
  'repository.discover',
  'refs.list',
  'blame.lines',
  'history.file',
  'search.run',
  'revision.content',
  'diff.file',
]

if (fixture.protocolVersion !== 1) throw new Error('protocolVersion must be 1')
if (fixture.transport !== 'ndjson-jsonrpc-2.0-stdio') throw new Error('unexpected transport')
if (fixture.readOnly !== true || fixture.network !== false) throw new Error('protocol must remain read-only and offline')
if (!Array.isArray(fixture.methods) || requiredMethods.some((method) => !fixture.methods.includes(method))) {
  throw new Error('canonical method allowlist is incomplete')
}
NODE
}

capture_helper_metadata() {
  local request_path="${TEMP_DIR}/initialize-request.ndjson"
  local response_path="${TEMP_DIR}/initialize-response.ndjson"
  local metadata_path="${TEMP_DIR}/helper-metadata.json"

  "${NODE_BIN}" - "${PROTOCOL_FIXTURE}" "${request_path}" <<'NODE'
const fs = require('node:fs')

const fixture = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'))
const request = {
  jsonrpc: '2.0',
  id: 'release-manifest',
  method: 'initialize',
  params: { protocolVersion: fixture.protocolVersion },
}
fs.writeFileSync(process.argv[3], `${JSON.stringify(request)}\n`, 'utf8')
NODE

  (
    cd "${ROOT_DIR}"
    "${GO_BIN}" run "${HELPER_PACKAGE}" <"${request_path}" >"${response_path}"
  )

  "${NODE_BIN}" - "${PROTOCOL_FIXTURE}" "${response_path}" "${metadata_path}" <<'NODE'
const fs = require('node:fs')

const fixture = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'))
const lines = fs.readFileSync(process.argv[3], 'utf8').split(/\r?\n/).filter((line) => line.trim() !== '')
if (lines.length !== 1) throw new Error(`helper initialize probe returned ${lines.length} response lines`)

const response = JSON.parse(lines[0])
if (response.jsonrpc !== '2.0' || response.id !== 'release-manifest' || response.error) {
  throw new Error('helper initialize probe returned an invalid JSON-RPC response')
}
const result = response.result
if (!result || result.protocolVersion !== fixture.protocolVersion || result.transport !== fixture.transport) {
  throw new Error('helper initialize protocol metadata does not match the canonical fixture')
}
if (result.readOnly !== true || result.network !== false ||
    result.capabilities?.readOnly !== true || result.capabilities?.network !== false) {
  throw new Error('helper initialize response violates the read-only/offline contract')
}
if (result.server?.name !== 'gitgit-vscode-helper' ||
    typeof result.server.version !== 'string' || result.server.version.trim() === '') {
  throw new Error('helper initialize response is missing server version metadata')
}

fs.writeFileSync(process.argv[4], `${JSON.stringify({
  helperServerVersion: result.server.version,
  protocolVersion: result.protocolVersion,
})}\n`, 'utf8')
NODE

  printf '%s' "${metadata_path}"
}

npm_has_script() {
  "${NODE_BIN}" -e \
    'const pkg=require(process.argv[1]); process.exit(pkg.scripts && pkg.scripts[process.argv[2]] ? 0 : 1)' \
    "${EXTENSION_DIR}/package.json" "$1"
}

extension_main_exists() {
  local extension_main
  extension_main="$("${NODE_BIN}" -e 'const pkg=require(process.argv[1]); process.stdout.write(pkg.main || "")' "${EXTENSION_DIR}/package.json")"
  [ -n "${extension_main}" ] || fail "VS Code package.json must declare a main entrypoint"
  extension_main="${extension_main#./}"
  [ -f "${EXTENSION_DIR}/${extension_main}" ] || fail "built extension entrypoint not found: ${extension_main}"
}

run_checks() {
  log "installing pinned VS Code extension dependencies"
  (
    cd "${EXTENSION_DIR}"
    "${NPM_BIN}" ci
  )

  validate_protocol_fixture

  [ "${EXTENSION_DIR}" != "/" ] || fail "refusing to clean an unsafe extension directory"
  log "removing stale generated extension outputs"
  rm -rf -- "${EXTENSION_DIR}/dist" "${EXTENSION_DIR}/dist-tests"

  log "running Go core and helper tests against protocol v1"
  (
    cd "${ROOT_DIR}"
    "${GO_BIN}" test ./...
  )

  log "running VS Code extension tests"
  (
    cd "${EXTENSION_DIR}"
    "${NPM_BIN}" test
    if npm_has_script check; then
      "${NPM_BIN}" run check
    elif npm_has_script typecheck; then
      "${NPM_BIN}" run typecheck
    else
      fail "package.json must define a check or typecheck script"
    fi
    "${NPM_BIN}" run build
  )

  extension_main_exists
}

target_settings() {
  case "$1" in
    darwin-arm64) printf 'darwin arm64 gitgit-vscode-helper' ;;
    darwin-x64) printf 'darwin amd64 gitgit-vscode-helper' ;;
    win32-x64) printf 'windows amd64 gitgit-vscode-helper.exe' ;;
    *) fail "unsupported VSIX target: $1" ;;
  esac
}

audit_vsix() {
  local artifact="$1"
  local target="$2"
  local expected_version="$3"
  local settings goos goarch helper_name expected_helper entries helper_entries helper_count banned_entries
  local manifest package_json extraction_dir extracted_helper build_info

  settings="$(target_settings "${target}")"
  set -- ${settings}
  goos="$1"
  goarch="$2"
  helper_name="$3"
  expected_helper="extension/dist/bin/${helper_name}"

  [ -s "${artifact}" ] || fail "VSIX was not created: ${artifact}"
  entries="$(unzip -Z1 "${artifact}")"

  helper_entries="$(printf '%s\n' "${entries}" | grep -E '^extension/dist/bin/gitgit-vscode-helper(\.exe)?$' || true)"
  helper_count="$(printf '%s\n' "${helper_entries}" | sed '/^$/d' | wc -l | tr -d ' ')"
  [ "${helper_count}" = "1" ] || fail "${target} VSIX must contain exactly one helper; found ${helper_count}"
  [ "${helper_entries}" = "${expected_helper}" ] || fail "${target} VSIX contains the wrong helper path: ${helper_entries}"
  printf '%s\n' "${entries}" | grep -F 'extension/LICENSE.txt' >/dev/null || fail "${target} VSIX is missing LICENSE.txt"
  printf '%s\n' "${entries}" | grep -F 'extension/readme.md' >/dev/null || fail "${target} VSIX is missing readme.md"

  banned_entries="$(printf '%s\n' "${entries}" | grep -E '(^|/)(src|test|tests|testdata|node_modules)(/|$)|\.map$|protocol-v1\.json$' || true)"
  [ -z "${banned_entries}" ] || fail "${target} VSIX contains forbidden development content:\n${banned_entries}"

  manifest="$(unzip -p "${artifact}" extension.vsixmanifest)"
  printf '%s' "${manifest}" | grep -F "TargetPlatform=\"${target}\"" >/dev/null || \
    fail "${target} is missing from extension.vsixmanifest TargetPlatform"

  package_json="${TEMP_DIR}/package-${target}.json"
  unzip -p "${artifact}" extension/package.json >"${package_json}"
  "${NODE_BIN}" - "${package_json}" "${expected_version}" <<'NODE'
const fs = require('node:fs')

const packagePath = process.argv[2]
const expectedVersion = process.argv[3]
const pkg = JSON.parse(fs.readFileSync(packagePath, 'utf8'))
if (`${pkg.publisher}.${pkg.name}` !== 'gitgit.gitgit') throw new Error('unexpected extension ID')
if (pkg.version !== expectedVersion) throw new Error(`unexpected extension version: ${pkg.version}`)
NODE

  extraction_dir="${TEMP_DIR}/extract-${target}"
  mkdir -p "${extraction_dir}"
  unzip -qq "${artifact}" "${expected_helper}" -d "${extraction_dir}"
  extracted_helper="${extraction_dir}/${expected_helper}"
  if [ "${goos}" = "darwin" ]; then
    [ -x "${extracted_helper}" ] || fail "${target} helper lost its executable mode in the VSIX"
  fi
  build_info="$("${GO_BIN}" version -m "${extracted_helper}")"
  printf '%s\n' "${build_info}" | grep -F "GOOS=${goos}" >/dev/null || fail "${target} helper GOOS audit failed"
  printf '%s\n' "${build_info}" | grep -F "GOARCH=${goarch}" >/dev/null || fail "${target} helper GOARCH audit failed"
  printf '%s\n' "${build_info}" | grep -F 'CGO_ENABLED=0' >/dev/null || fail "${target} helper must be built with CGO_ENABLED=0"

  log "audited ${artifact} (${target}, one ${goos}/${goarch} helper, protocol fixture excluded)"
}

package_target() {
  local target="$1"
  local version="$2"
  local settings goos goarch helper_name helper_dir helper_path artifact vsce_bin

  settings="$(target_settings "${target}")"
  set -- ${settings}
  goos="$1"
  goarch="$2"
  helper_name="$3"
  helper_dir="${EXTENSION_DIR}/dist/bin"
  helper_path="${helper_dir}/${helper_name}"
  artifact="${OUTPUT_DIR}/gitgit-${version}-${target}.vsix"
  vsce_bin="${EXTENSION_DIR}/node_modules/.bin/vsce"

  [ -x "${vsce_bin}" ] || fail "pinned vsce binary not found; run without --skip-checks first"
  mkdir -p "${helper_dir}" "${OUTPUT_DIR}"
  rm -f -- "${helper_dir}/gitgit-vscode-helper" "${helper_dir}/gitgit-vscode-helper.exe" "${artifact}"

  log "cross-building helper for ${target} with CGO_ENABLED=0"
  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      "${GO_BIN}" build -trimpath -o "${helper_path}" "${HELPER_PACKAGE}"
  )
  if [ "${goos}" = "darwin" ]; then
    chmod 0755 "${helper_path}"
  fi

  log "packaging ${target} VSIX"
  (
    cd "${EXTENSION_DIR}"
    "${vsce_bin}" package --no-dependencies --target "${target}" --out "${artifact}"
  )
  audit_vsix "${artifact}" "${target}" "${version}"
}

write_release_manifest() {
  local version="$1"
  local helper_metadata_path="$2"
  local targets="$3"
  local manifest_path="${OUTPUT_DIR}/release-manifest.json"

  mkdir -p "${OUTPUT_DIR}"
  "${NODE_BIN}" - \
    "${OUTPUT_DIR}" \
    "${version}" \
    "${helper_metadata_path}" \
    "${targets}" \
    "${manifest_path}" <<'NODE'
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')

const outputDirectory = process.argv[2]
const extensionVersion = process.argv[3]
const helperMetadata = JSON.parse(fs.readFileSync(process.argv[4], 'utf8'))
const requestedTargets = new Set(process.argv[5].trim().split(/\s+/).filter(Boolean))
const manifestPath = process.argv[6]
const canonicalTargets = ['darwin-arm64', 'darwin-x64', 'win32-x64']
const targets = canonicalTargets.filter((target) => requestedTargets.has(target))

if (targets.length !== requestedTargets.size || targets.length === 0) {
  throw new Error('release manifest targets must be a non-empty subset of the v0.1 target allowlist')
}
if (typeof helperMetadata.helperServerVersion !== 'string' || helperMetadata.helperServerVersion === '') {
  throw new Error('helperServerVersion is missing')
}
if (!Number.isInteger(helperMetadata.protocolVersion) || helperMetadata.protocolVersion < 1) {
  throw new Error('protocolVersion is invalid')
}

const artifacts = targets.map((target) => {
  const file = `gitgit-${extensionVersion}-${target}.vsix`
  const artifactPath = path.join(outputDirectory, file)
  if (!fs.statSync(artifactPath).isFile()) throw new Error(`VSIX is missing: ${file}`)
  return {
    target,
    file,
    sha256: crypto.createHash('sha256').update(fs.readFileSync(artifactPath)).digest('hex'),
  }
})

const manifest = {
  schemaVersion: 1,
  extensionId: 'gitgit.gitgit',
  extensionVersion,
  helperServerVersion: helperMetadata.helperServerVersion,
  protocolVersion: helperMetadata.protocolVersion,
  artifacts,
}
const temporaryPath = `${manifestPath}.tmp`
fs.writeFileSync(temporaryPath, `${JSON.stringify(manifest, null, 2)}\n`, 'utf8')
fs.renameSync(temporaryPath, manifestPath)
NODE
}

audit_release_manifest() {
  local version="$1"
  local helper_metadata_path="$2"
  local targets="$3"
  local manifest_path="${OUTPUT_DIR}/release-manifest.json"

  "${NODE_BIN}" - \
    "${OUTPUT_DIR}" \
    "${version}" \
    "${helper_metadata_path}" \
    "${targets}" \
    "${manifest_path}" \
    "${EXTENSION_DIR}/package.json" \
    "${PROTOCOL_FIXTURE}" <<'NODE'
const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')

const outputDirectory = process.argv[2]
const expectedExtensionVersion = process.argv[3]
const helperMetadata = JSON.parse(fs.readFileSync(process.argv[4], 'utf8'))
const requestedTargets = new Set(process.argv[5].trim().split(/\s+/).filter(Boolean))
const manifestPath = process.argv[6]
const extensionPackage = JSON.parse(fs.readFileSync(process.argv[7], 'utf8'))
const protocolFixture = JSON.parse(fs.readFileSync(process.argv[8], 'utf8'))
const canonicalTargets = ['darwin-arm64', 'darwin-x64', 'win32-x64']
const targets = canonicalTargets.filter((target) => requestedTargets.has(target))
const manifestText = fs.readFileSync(manifestPath, 'utf8')
const manifest = JSON.parse(manifestText)

if (manifest.schemaVersion !== 1) throw new Error('release manifest schemaVersion must be 1')
if (manifest.extensionId !== `${extensionPackage.publisher}.${extensionPackage.name}` ||
    manifest.extensionId !== 'gitgit.gitgit') {
  throw new Error('release manifest extensionId does not match package.json')
}
if (manifest.extensionVersion !== expectedExtensionVersion ||
    manifest.extensionVersion !== extensionPackage.version) {
  throw new Error('release manifest extensionVersion does not match package.json')
}
if (manifest.helperServerVersion !== helperMetadata.helperServerVersion) {
  throw new Error('release manifest helperServerVersion does not match helper initialize')
}
if (manifest.protocolVersion !== helperMetadata.protocolVersion ||
    manifest.protocolVersion !== protocolFixture.protocolVersion) {
  throw new Error('release manifest protocolVersion does not match helper and fixture')
}
if (!Array.isArray(manifest.artifacts) || manifest.artifacts.length !== targets.length) {
  throw new Error('release manifest artifact count does not match this package invocation')
}

const expectedArtifacts = targets.map((target) => {
  const file = `gitgit-${expectedExtensionVersion}-${target}.vsix`
  const artifactPath = path.join(outputDirectory, file)
  const sha256 = crypto.createHash('sha256').update(fs.readFileSync(artifactPath)).digest('hex')
  return { target, file, sha256 }
})
const expected = {
  schemaVersion: 1,
  extensionId: 'gitgit.gitgit',
  extensionVersion: expectedExtensionVersion,
  helperServerVersion: helperMetadata.helperServerVersion,
  protocolVersion: protocolFixture.protocolVersion,
  artifacts: expectedArtifacts,
}
const expectedText = `${JSON.stringify(expected, null, 2)}\n`
if (manifestText !== expectedText) {
  throw new Error('release manifest is not in canonical deterministic form or contains stale metadata')
}
NODE

  log "audited ${manifest_path} for targets: ${targets}"
}

if [ "${RUN_CHECKS}" -eq 1 ]; then
  run_checks
else
  extension_main_exists
fi

log "reading helper server metadata from its initialize response"
helper_metadata_path="$(capture_helper_metadata)"

if [ "${CHECK_ONLY}" -eq 1 ]; then
  log "checks and helper metadata validation completed"
  exit 0
fi

[ -n "${TARGETS}" ] || TARGETS="${SUPPORTED_TARGETS}"
version="$("${NODE_BIN}" -e 'const pkg=require(process.argv[1]); process.stdout.write(pkg.version)' "${EXTENSION_DIR}/package.json")"
[ -n "${version}" ] || fail "extension version is empty"

mkdir -p "${OUTPUT_DIR}"
rm -f -- "${OUTPUT_DIR}/release-manifest.json" "${OUTPUT_DIR}/release-manifest.json.tmp"

for target in ${TARGETS}; do
  package_target "${target}" "${version}"
done

write_release_manifest "${version}" "${helper_metadata_path}" "${TARGETS}"
audit_release_manifest "${version}" "${helper_metadata_path}" "${TARGETS}"

log "VSIX artifacts and release-manifest.json are available under ${OUTPUT_DIR}"
