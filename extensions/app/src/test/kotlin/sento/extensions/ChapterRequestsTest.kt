package sento.extensions

import eu.kanade.tachiyomi.source.model.SChapter
import eu.kanade.tachiyomi.source.model.SManga
import eu.kanade.tachiyomi.source.model.SMangaUpdate
import eu.kanade.tachiyomi.source.online.HttpSource
import java.io.IOException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class ChapterRequestsTest {
    private fun manga(url: String, name: String = "Example") = SManga.create().apply {
        this.url = url
        title = name
    }

    private fun source(id: Long = 1, update: suspend (SManga) -> List<SChapter>) = object : HttpSource() {
        override val id = id
        override val name = "Fixture"
        override val lang = "en"
        override val supportsLatest = false
        override val baseUrl = "https://manga.example"
        override suspend fun getMangaUpdate(
            manga: SManga,
            chapters: List<SChapter>,
            fetchDetails: Boolean,
            fetchChapters: Boolean,
        ) = SMangaUpdate(manga, update(manga))
    }

    @Test
    fun `overlapping requests share an update even when their titles differ`() = runBlocking {
        val requests = ChapterRequests()
        val started = CompletableDeferred<Unit>()
        val finish = CompletableDeferred<Unit>()
        val chapters = listOf(SChapter.create().apply { name = "Chapter 1"; url = "/one/1" })
        var calls = 0
        val source = source {
            calls++
            started.complete(Unit)
            finish.await()
            chapters
        }
        val first = async { requests.chapters(source, manga("/one")) }
        started.await()
        val second = async(start = CoroutineStart.UNDISPATCHED) {
            requests.chapters(source, manga("/one", "Another title"))
        }
        try {
            assertEquals(1, calls)
        } finally {
            finish.complete(Unit)
        }
        assertEquals(chapters, first.await())
        assertEquals(chapters, second.await())
        // A subsequent request must still discover new chapters.
        assertEquals(chapters, requests.chapters(source, manga("/one")))
        assertEquals(2, calls)
    }

    @Test
    fun `another manga or source does not wait for the first update`() = runBlocking {
        withTimeout(5_000) {
            val requests = ChapterRequests()
            val started = CompletableDeferred<Unit>()
            val finish = CompletableDeferred<Unit>()
            val source = source {
                if (it.url == "/one") {
                    started.complete(Unit)
                    finish.await()
                }
                emptyList()
            }
            val first = async { requests.chapters(source, manga("/one")) }
            started.await()
            try {
                assertEquals(emptyList(), requests.chapters(source, manga("/two")))
                assertEquals(emptyList(), requests.chapters(source(2) { emptyList() }, manga("/one")))
            } finally {
                finish.complete(Unit)
            }
            first.await()
            Unit
        }
    }

    @Test
    fun `a failed update can be retried`() = runBlocking {
        val requests = ChapterRequests()
        var calls = 0
        val source = source {
            if (++calls == 1) throw IOException("temporary source failure")
            emptyList()
        }
        assertFailsWith<IOException> { requests.chapters(source, manga("/one")) }
        assertEquals(emptyList(), requests.chapters(source, manga("/one")))
        assertEquals(2, calls)
    }

    @Test
    fun `cancelling a waiting request does not cancel the shared update`() = runBlocking {
        val requests = ChapterRequests()
        val started = CompletableDeferred<Unit>()
        val finish = CompletableDeferred<Unit>()
        var calls = 0
        val source = source {
            calls++
            started.complete(Unit)
            finish.await()
            emptyList()
        }
        val first = async { requests.chapters(source, manga("/one")) }
        started.await()
        val waiter = async(start = CoroutineStart.UNDISPATCHED) { requests.chapters(source, manga("/one")) }
        waiter.cancel()
        waiter.join()
        finish.complete(Unit)
        assertEquals(emptyList(), first.await())
        assertEquals(1, calls)
    }
}
