#!/usr/bin/env bash
# Installs git hooks from scripts/ into .git/hooks/.
# Run once after cloning: bash scripts/install-hooks.sh

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"

install_hook() {
  local name="$1"
  local src="$REPO_ROOT/scripts/$name"
  local dst="$HOOKS_DIR/$name"

  if [ ! -f "$src" ]; then
    echo "Hook script not found: $src" >&2
    return 1
  fi

  ln -sf "$src" "$dst"
  chmod +x "$src"
  echo "Installed $name -> $dst"
}

install_hook pre-commit

echo "Done. Hooks installed in $HOOKS_DIR."
