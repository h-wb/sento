#!/bin/sh
# Vendors the runtime shim layers from Suwayomi-Server (MPL-2.0;
# file-level copyleft — vendored files keep their headers and stay under
# MPL). Only the files that are NOT a standalone module upstream are
# copied: the eu.kanade.tachiyomi API surface extensions compile against
# and the APK loading machinery, both entangled with Suwayomi's server
# tree upstream and patched here via the suwayomi.tachidesk.server
# stand-ins in app/. The AndroidCompat module builds straight from the
# Suwayomi-Server git submodule and is never copied.
#
# Usage: host/scripts/vendor.sh   (after bumping the submodule pin)
set -eu

HOST="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${1:-$HOST/Suwayomi-Server}"
KT="$SRC/server/src/main/kotlin"

[ -d "$KT" ] || { echo "Suwayomi-Server checkout not found at $SRC (git submodule update --init?)" >&2; exit 1; }

# The eu.kanade.tachiyomi runtime extensions compile against. The local
# source (filesystem manga) is Suwayomi-specific and excluded.
rsync -a --delete --exclude 'source/local/' --exclude 'App.kt' --exclude 'AppModule.kt' \
  --exclude 'util/chapter/ChapterSanitizer.kt' --exclude 'util/storage/EpubFile.kt' --exclude 'util/lang/StringExtensions.kt' \
  "$KT/eu/kanade/tachiyomi/" "$HOST/app/src/main/kotlin/eu/kanade/tachiyomi/"

# Extension loading machinery + the util helpers the runtime needs.
APP="$HOST/app/src/main/kotlin/suwayomi/tachidesk/manga/impl"
mkdir -p "$APP/util/source" "$APP/extension/github"
for f in util/PackageTools.kt util/BytecodeEditor.kt util/ChildFirstURLClassLoader.kt; do
  cp "$KT/suwayomi/tachidesk/manga/impl/$f" "$APP/$f"
done
rsync -a --delete --exclude ExposedExtensions.kt "$KT/suwayomi/tachidesk/manga/impl/util/lang/" "$APP/util/lang/"
rsync -a --delete "$KT/suwayomi/tachidesk/manga/impl/util/network/" "$APP/util/network/"
rsync -a --delete "$KT/suwayomi/tachidesk/manga/impl/util/storage/" "$APP/util/storage/"
cp "$KT/suwayomi/tachidesk/manga/impl/extension/github/ExtensionGithubApi.kt" \
   "$KT/suwayomi/tachidesk/manga/impl/extension/github/OnlineExtension.kt" \
   "$APP/extension/github/"

echo "vendored shim layer from $SRC"
