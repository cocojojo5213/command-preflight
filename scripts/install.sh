#!/usr/bin/env sh
set -eu

repository="${COMMAND_PREFLIGHT_REPO:-cocojojo5213/command-preflight}"
prefix="${COMMAND_PREFLIGHT_PREFIX:-$HOME/.local/bin}"
case "$(uname -s)" in
  Linux) platform="linux" ;;
  Darwin) platform="darwin" ;;
  *) printf '%s\n' 'Unsupported platform. Use install.ps1 on Windows.' >&2; exit 2 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *) printf 'Unsupported architecture: %s\n' "$(uname -m)" >&2; exit 2 ;;
esac

asset="command-preflight_${platform}_${architecture}.tar.gz"
base_url="https://github.com/${repository}/releases/latest/download"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

download() {
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error --location "$1" --output "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget --quiet --output-document="$2" "$1"
  else
    printf '%s\n' 'curl or wget is required.' >&2
    exit 1
  fi
}

download "${base_url}/${asset}" "${temporary_dir}/${asset}"
mkdir -p "$temporary_dir/unpack" "$prefix"
tar -xzf "${temporary_dir}/${asset}" -C "$temporary_dir/unpack"
install -m 0755 "${temporary_dir}/unpack/command-preflight" "${prefix}/command-preflight"

"${prefix}/command-preflight" install-skill --target both

if ! "${prefix}/command-preflight" setup --client both --apply; then
  printf '%s\n' 'Binary installed, but automatic MCP registration needs to be completed manually.' >&2
fi
printf '\nInstalled %s/command-preflight\n' "$prefix"
if [ -n "${COMMAND_PREFLIGHT_KNOWLEDGE_URL:-}" ]; then
  printf '%s\n' "Opt-in knowledge lookup configured for: ${COMMAND_PREFLIGHT_KNOWLEDGE_URL}"
else
  printf '%s\n' 'Knowledge lookup remains offline (set COMMAND_PREFLIGHT_KNOWLEDGE_URL to opt in).'
fi
if [ "${COMMAND_PREFLIGHT_REPORTING:-off}" = "on" ] || [ "${COMMAND_PREFLIGHT_REPORTING:-off}" = "true" ]; then
  printf '%s\n' "Opt-in moderated reporting configured for: ${COMMAND_PREFLIGHT_REPORT_URL:-${COMMAND_PREFLIGHT_KNOWLEDGE_URL:-unset}}"
else
  printf '%s\n' 'Community reporting remains disabled (set COMMAND_PREFLIGHT_REPORTING=on to opt in).'
fi
printf '%s\n' 'If the command is not found, add the prefix to PATH for your shell.'
