#!/usr/bin/env sh
set -eu

REPO_OWNER="alfariiizi"
REPO_NAME="vandor"
BINARY_NAME="vandor"

VERSION="${VANDOR_VERSION:-latest}"
INSTALL_DIR="${VANDOR_INSTALL_DIR:-}"

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

need_cmd curl
need_cmd tar
need_cmd mktemp
need_cmd uname

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux) echo "linux" ;;
    darwin) echo "darwin" ;;
    *) fail "unsupported OS: $os (supported: linux, darwin, WSL)" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) fail "unsupported architecture: $arch (supported: amd64, arm64)" ;;
  esac
}

resolve_latest_tag() {
  need_cmd sed
  url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/latest")"
  tag="$(printf '%s' "$url" | sed 's#.*/tag/##')"
  [ -n "$tag" ] || fail "failed to resolve latest release tag"
  printf '%s' "$tag"
}

is_wsl() {
  if [ "$(detect_os)" != "linux" ]; then
    return 1
  fi
  if grep -qi microsoft /proc/version 2>/dev/null; then
    return 0
  fi
  return 1
}

if [ "$VERSION" = "latest" ]; then
  VERSION="$(resolve_latest_tag)"
fi

version_trimmed="$(printf '%s' "$VERSION" | sed 's/^v//')"
os="$(detect_os)"
arch="$(detect_arch)"
archive="${BINARY_NAME}_${version_trimmed}_${os}_${arch}.tar.gz"
download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${archive}"

if [ -z "$INSTALL_DIR" ]; then
  if [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

log "Installing ${BINARY_NAME} ${VERSION} (${os}/${arch})"
log "Download: ${download_url}"
log "Install dir: ${INSTALL_DIR}"

curl -fL "$download_url" -o "${tmp_dir}/${archive}" || fail "download failed"
tar -xzf "${tmp_dir}/${archive}" -C "$tmp_dir" || fail "extract failed"
[ -f "${tmp_dir}/${BINARY_NAME}" ] || fail "binary not found in archive"

mkdir -p "$INSTALL_DIR" || fail "cannot create install dir: ${INSTALL_DIR}"
install_path="${INSTALL_DIR}/${BINARY_NAME}"
cp "${tmp_dir}/${BINARY_NAME}" "$install_path" || fail "copy failed"
chmod +x "$install_path" || fail "chmod failed"

log ""
log "Installed: ${install_path}"
log ""

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    log "PATH already includes ${INSTALL_DIR}"
    ;;
  *)
    log "Add this to your shell profile:"
    log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac

log ""
log "Optional official registry env:"
log "  export VANDOR_VPKG_REGISTRY_OFFICIAL=https://vpkg.vercel.app"
log ""
if is_wsl; then
  log "WSL detected."
fi
log "Verify:"
log "  ${BINARY_NAME} --version"
