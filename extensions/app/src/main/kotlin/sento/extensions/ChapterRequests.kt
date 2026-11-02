package sento.extensions

import eu.kanade.tachiyomi.source.Source
import eu.kanade.tachiyomi.source.model.SChapter
import eu.kanade.tachiyomi.source.model.SManga
import kotlinx.coroutines.CompletableDeferred

// Extensions may keep mutable state while updating a manga. Share an
// overlapping chapter request instead of entering the extension twice.
// Results and failures are discarded once the request finishes.
internal class ChapterRequests {
    private data class Key(val source: Long, val url: String)

    private val pending = mutableMapOf<Key, CompletableDeferred<List<SChapter>>>()

    suspend fun chapters(source: Source, manga: SManga): List<SChapter> {
        val key = Key(source.id, manga.url)
        var owner = false
        val result = synchronized(pending) {
            pending.getOrPut(key) {
                owner = true
                CompletableDeferred()
            }
        }
        if (!owner) return result.await()
        try {
            val chapters = source.getMangaUpdate(manga, emptyList(), fetchDetails = false, fetchChapters = true).chapters
            result.complete(chapters)
            return chapters
        } catch (error: Throwable) {
            result.completeExceptionally(error)
            throw error
        } finally {
            synchronized(pending) { pending.remove(key) }
        }
    }
}
