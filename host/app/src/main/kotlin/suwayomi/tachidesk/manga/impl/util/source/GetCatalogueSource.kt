// Slim replacement for Suwayomi's DB-backed source registry, keeping the
// upstream object name so vendored files (NetworkHelper) compile
// unchanged. Sources are registered by the ExtensionManager at startup
// and after installs; there is no database.
package suwayomi.tachidesk.manga.impl.util.source

import eu.kanade.tachiyomi.source.CatalogueSource
import java.util.concurrent.ConcurrentHashMap

object GetCatalogueSource {
    private val sourceCache = ConcurrentHashMap<Long, CatalogueSource>()

    // reloader is installed by the ExtensionManager: re-instantiates all
    // sources from the installed jars (after a user-agent change clears
    // the cache, headers must be rebuilt).
    @Volatile
    var reloader: (() -> Unit)? = null

    fun getCatalogueSourceOrNull(sourceId: Long): CatalogueSource? {
        if (sourceCache.isEmpty()) reloader?.invoke()
        return sourceCache[sourceId]
    }

    fun all(): List<CatalogueSource> {
        if (sourceCache.isEmpty()) reloader?.invoke()
        return sourceCache.values.sortedBy { it.name }
    }

    fun registerCatalogueSource(sourcePair: Pair<Long, CatalogueSource>) {
        sourceCache += sourcePair
    }

    fun unregisterCatalogueSource(sourceId: Long) {
        sourceCache.remove(sourceId)
    }

    fun unregisterAllCatalogueSources() {
        sourceCache.clear()
    }
}
