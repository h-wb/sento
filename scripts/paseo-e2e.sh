#!/usr/bin/env bash
# The Paseo agent image has Chromium but no system browser libraries or sudo.
# Reuse its shared, extracted Debian packages. See docs/testing/paseo.md.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
browser_deps="${SENTO_PASEO_BROWSER_DEPS:-${XDG_CACHE_HOME:-$HOME/.cache}/sento-playwright-libs}"

for required in root/usr/lib/x86_64-linux-gnu/libglib-2.0.so.0 root/lib/x86_64-linux-gnu/libdbus-1.so.3 fonts.conf; do
  if [[ ! -f "$browser_deps/$required" ]]; then
    echo "Paseo's browser dependency cache is missing: $browser_deps/$required" >&2
    echo "See docs/testing/paseo.md for the environment setup; ordinary hosts use mise run e2e:install." >&2
    exit 1
  fi
done

# Debian splits these libraries between /usr/lib and /lib. Both are needed;
# using only /usr/lib still leaves Chromium unable to load libdbus-1.so.3.
export LD_LIBRARY_PATH="$browser_deps/root/usr/lib/x86_64-linux-gnu:$browser_deps/root/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export FONTCONFIG_FILE="$browser_deps/fonts.conf"

# Keep builds, pinned tools, process cleanup and Playwright arguments on the
# standard path. The cache applies only to this task and its children.
exec mise -C "$repo_root" run e2e -- "$@"
