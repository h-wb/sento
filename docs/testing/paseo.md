# Browser tests in the Paseo agent environment

This workaround is specific to the unprivileged Linux x86-64 Paseo agent
image. Ordinary development and CI use `mise run e2e:install` followed by
`mise run e2e`, as described in the [testing guide](README.md).

The agent image has Playwright's Chromium download, but lacks system browser
libraries. Chromium fails before any test runs with errors such as
`libglib-2.0.so.0: cannot open shared object file`. Installing dependencies
with `mise run e2e:install` tries to become root and fails because the agent
has no administrator credentials.

Use the hidden task instead:

```sh
mise run e2e:paseo
mise run e2e:paseo -- blocklist.spec.ts
mise run e2e:paseo -- acquisition.spec.ts activity.spec.ts --grep replacement
```

The task reuses the environment's shared dependency cache at
`${XDG_CACHE_HOME:-$HOME/.cache}/sento-playwright-libs`. Extracted Debian 12
packages live under `root/`, with libraries in **both**
`root/usr/lib/x86_64-linux-gnu` and `root/lib/x86_64-linux-gnu`. The second
directory contains `libdbus-1.so.3`; omitting it causes another launch failure.
The cache also supplies `fonts.conf`, which points to its extracted fonts
and writable font cache.

The wrapper sets `LD_LIBRARY_PATH` and `FONTCONFIG_FILE` only for the test
process, then delegates to the standard `e2e` task. Builds, toolchain pins,
fixture servers, port selection and cleanup stay the same. The cache is
shared across worktrees; do not copy it into the repository or change the
normal Playwright configuration for this environment.

If the cache is missing or incomplete, the wrapper reports the missing path.
Restore the environment's extracted dependency cache before retrying. If it
has been provisioned elsewhere, set `SENTO_PASEO_BROWSER_DEPS` in gitignored
`mise.local.toml`; its `fonts.conf` must reference that cache's actual fonts
and writable font-cache directory. Do not retry the root installer or put
administrator credentials in the checkout. This task reuses the cache; it
does not download or install system packages.
