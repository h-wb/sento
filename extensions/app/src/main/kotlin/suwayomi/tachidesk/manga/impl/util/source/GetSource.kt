// Slim replacement for Suwayomi's DB-backed source registry, keeping the
// upstream object and method names so vendored files (NetworkHelper)
// compile unchanged. Upstream renamed this from GetCatalogueSource to
// GetSource in v2.3. Sources are registered by the ExtensionManager at startup
// and after installs; there is no database.
package suwayomi.tachidesk.manga.impl.util.source

import eu.kanade.tachiyomi.source.CatalogueSource
import java.util.concurrent.ConcurrentHashMap

object GetSource {
    private val sourceCache = ConcurrentHashMap<Long, CatalogueSource>()

    // reloader is installed by the ExtensionManager: re-instantiates all
    // sources from the installed jars (after a user-agent change clears
    // the cache, headers must be rebuilt).
    @Volatile
    var reloader: (() -> Unit)? = null

    fun getSourceOrNull(sourceId: Long): CatalogueSource? {
        sourceCache[sourceId]?.let { return it }
        // Miss: a preference change bounced the instance (or nothing is
        // loaded yet) — re-instantiate from the installed jars.
        reloader?.invoke()
        return sourceCache[sourceId]
    }

    fun all(): List<CatalogueSource> {
        if (sourceCache.isEmpty()) reloader?.invoke()
        return sourceCache.values.sortedBy { it.name }
    }

    fun registerSource(sourcePair: Pair<Long, CatalogueSource>) {
        sourceCache += sourcePair
    }

    fun unregisterSource(sourceId: Long) {
        sourceCache.remove(sourceId)
    }

    // Preference changes must rebuild the instance immediately. Removing
    // just this source leaves all() with a nonempty, incomplete registry,
    // so discovery would never ask for the missing source to reload it.
    fun reloadSource(sourceId: Long) {
        unregisterSource(sourceId)
        getSourceOrNull(sourceId)
    }

    fun unregisterAllSources() {
        sourceCache.clear()
    }
}
