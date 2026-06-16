#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF' >&2
usage: scripts/update.sh <version-without-leading-v>

Required environment:
  HOMEBREW_TAP_PATH   Absolute path to the homebrew-tap repository

Optional environment:
  GIT_REMOTE          Git remote to push the tag to (default: origin)
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 1
fi

version="$1"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${repo_root}/.env"
git_remote="${GIT_REMOTE:-origin}"
tag="v${version}"

load_env_file() {
  local path="$1"
  [[ -f "$path" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*$ ]] && continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local value="${BASH_REMATCH[2]}"
      value="${value%\"}"
      value="${value#\"}"
      value="${value%\'}"
      value="${value#\'}"
      export "${key}=${value}"
    fi
  done < "$path"
}

load_env_file "$env_file"

if [[ -z "${HOMEBREW_TAP_PATH:-}" ]]; then
  echo "HOMEBREW_TAP_PATH is required. Set it in ${env_file}." >&2
  exit 1
fi

if [[ ! -d "$HOMEBREW_TAP_PATH/.git" ]]; then
  echo "HOMEBREW_TAP_PATH does not point to a git repository: $HOMEBREW_TAP_PATH" >&2
  exit 1
fi

if ! git -C "$repo_root" diff --quiet || ! git -C "$repo_root" diff --cached --quiet; then
  echo "Working tree is dirty. Commit or stash changes before tagging." >&2
  exit 1
fi

if git -C "$repo_root" rev-parse -q --verify "$tag" >/dev/null; then
  echo "Tag already exists: $tag"
else
  git -C "$repo_root" tag -a "$tag" -m "Release ${tag}"
fi

git -C "$repo_root" push "$git_remote" "$tag"

tap_formula="${HOMEBREW_TAP_PATH}/Formula/slash-key.rb"
tap_commit_message="Update slash-key to ${tag}"

if [[ ! -f "$tap_formula" ]]; then
  echo "Formula file not found: $tap_formula" >&2
  exit 1
fi

url="https://github.com/kmatsushita1012/slash-key/archive/refs/tags/${tag}.tar.gz"
sha256="$(curl -L --fail --silent --show-error "$url" | shasum -a 256 | awk '{print $1}')"

python3 - "$tap_formula" "$url" "$sha256" <<'PY'
from pathlib import Path
import re
import sys

formula_path = Path(sys.argv[1])
url = sys.argv[2]
sha256 = sys.argv[3]

text = formula_path.read_text()
text, count_url = re.subn(
    r'url "https://github\.com/kmatsushita1012/slash-key/archive/refs/tags/v[^"]+\.tar\.gz"',
    f'url "{url}"',
    text,
)
text, count_sha = re.subn(
    r'sha256 "[0-9a-f]{64}"',
    f'sha256 "{sha256}"',
    text,
)

if count_url != 1 or count_sha != 1:
    raise SystemExit("failed to update formula contents")

formula_path.write_text(text)
PY

git -C "$HOMEBREW_TAP_PATH" add Formula/slash-key.rb

if git -C "$HOMEBREW_TAP_PATH" diff --cached --quiet; then
  echo "No tap changes to commit."
  exit 0
fi

git -C "$HOMEBREW_TAP_PATH" commit -m "$tap_commit_message"
git -C "$HOMEBREW_TAP_PATH" push origin main

echo "Published ${tag} and updated Homebrew tap at ${HOMEBREW_TAP_PATH}"
