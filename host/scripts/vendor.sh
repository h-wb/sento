#!/bin/sh
# Vendors the extension-runtime layers from a Suwayomi-Server checkout
# (MPL-2.0; file-level copyleft — vendored files keep their headers and
# stay under MPL). Re-run against a newer checkout to pick up upstream
# fixes, then re-apply local patches if any fail to compile.
#
# Usage: host/scripts/vendor.sh [path-to-Suwayomi-Server]
set -eu

SRC="${1:-../Suwayomi-Server}"
HOST="$(cd "$(dirname "$0")/.." && pwd)"
KT="$SRC/server/src/main/kotlin"

[ -d "$KT" ] || { echo "Suwayomi-Server checkout not found at $SRC" >&2; exit 1; }

# The fake Android runtime (standalone module, no server imports).
rsync -a --delete "$SRC/AndroidCompat/src/" "$HOST/AndroidCompat/src/"
rsync -a --delete "$SRC/AndroidCompat/Config/src/" "$HOST/AndroidCompat/Config/src/"

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

# Dependency versions follow upstream so vendored code keeps compiling.
cp "$SRC/gradle/libs.versions.toml" "$HOST/gradle/libs.versions.toml"

echo "vendored from $SRC"
