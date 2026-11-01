#!/usr/bin/env bash
set -euo pipefail

# Paseo supplies the source checkout for local configuration. Copy only
# explicitly supported, ignored files; preserve any destination overrides.
if [ -n "${PASEO_SOURCE_CHECKOUT_PATH:-}" ]; then
  for file in mise.local.toml fnox.toml; do
    if [ -f "$PASEO_SOURCE_CHECKOUT_PATH/$file" ] && [ ! -e "$file" ] && git check-ignore -q -- "$file"; then
      cp -p "$PASEO_SOURCE_CHECKOUT_PATH/$file" "$file"
    fi
  done
fi

mise install
mise run web:install
