# Third-party notices

sento-extensions exists because of other people's work. It does not
reimplement an extension runtime: it runs the one built by the Suwayomi
project, and through it the extension API of the Tachiyomi and Mihon
projects.

## Suwayomi-Server

- Project: <https://github.com/Suwayomi/Suwayomi-Server>
- Copyright (C) Contributors to the Suwayomi project
- License: Mozilla Public License 2.0 (`LICENSES/MPL-2.0.txt`)

What sento-extensions takes from it, at the release pinned in
[`vendir.yml`](vendir.yml) and locked in `vendir.lock.yml`:

- **AndroidCompat** (and its Config module): the fake Android runtime that
  lets extension APKs, written for Android, run on a plain JVM.
- **The extension API surface and APK loading machinery**: the
  `eu.kanade.tachiyomi` packages, `PackageTools`, `BytecodeEditor`,
  `ChildFirstURLClassLoader`, the network, storage and language helpers,
  and the extension-repository index readers.

These files are compiled into sento-extensions **unmodified**. They are
not copied into this repository: [vendir](https://carvel.dev/vendir/)
fetches them from the upstream repository at build time, each with its
original license header. The exact list of files is the `includePaths` /
`excludePaths` in `vendir.yml`.

Three files in this repository are adapted from Suwayomi-Server's source and
are therefore themselves under the MPL 2.0, as their headers state:

- `app/src/main/kotlin/sento/extensions/ExtensionManager.kt`, adapted from
  Suwayomi-Server's `Extension.kt` (install and uninstall logic, with the
  database replaced by a JSON index).
- `app/src/main/kotlin/eu/kanade/tachiyomi/network/HttpException.kt` and
  `OkHttpExtensions.kt`, adapted from the same upstream paths to retain
  HTTP `Retry-After` metadata when an extension request fails. Existing
  helper signatures and the one-argument exception constructor are preserved.

The files under `app/src/main/kotlin/suwayomi/` are small stand-ins
written for this project. They reuse Suwayomi's package and symbol names
only so that the vendored files compile without being patched.

### Getting the source

Binary builds of sento-extensions (including the container image) contain
compiled Suwayomi-Server code. Its source is available, under the MPL
2.0, from the upstream repository at the tag named in `vendir.yml`:

    https://github.com/Suwayomi/Suwayomi-Server

The source of sento-extensions itself, including the MPL-covered files
above, is in the `extensions/` directory of
<https://github.com/h-wb/sento>.

## Tachiyomi and Mihon

- Projects: <https://github.com/tachiyomiorg> (archived),
  <https://github.com/mihonapp/mihon>
- License: Apache License 2.0

The extension API that Suwayomi-Server carries, and that every extension
is written against, originates in Tachiyomi and continues in Mihon.
sento-extensions ships no extensions and no extension repositories;
users add the repositories they choose.

## Other dependencies

Runtime libraries (OkHttp, Javalin, kotlinx, dex2jar and others) are
declared in Suwayomi-Server's dependency catalog and in
`app/build.gradle.kts`, and are used under their own licenses.
