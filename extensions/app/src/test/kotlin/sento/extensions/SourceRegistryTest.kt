package sento.extensions

import eu.kanade.tachiyomi.source.online.HttpSource
import suwayomi.tachidesk.manga.impl.util.source.GetSource
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertSame

class SourceRegistryTest {
    private fun source(id: Long, name: String) = object : HttpSource() {
        override val id = id
        override val name = name
        override val lang = "en"
        override val supportsLatest = false
        override val baseUrl = "https://manga.example"
    }

    @AfterTest
    fun cleanRegistry() {
        GetSource.reloader = null
        GetSource.unregisterAllSources()
    }

    @Test
    fun `a preference reload keeps the changed source discoverable beside another extension`() {
        val original = source(1, "Original configuration")
        val other = source(2, "Other extension")
        GetSource.registerSource(original.id to original)
        GetSource.registerSource(other.id to other)
        val configured = source(1, "New configuration")
        var reloads = 0
        GetSource.reloader = {
            reloads++
            GetSource.registerSource(configured.id to configured)
            GetSource.registerSource(other.id to other)
        }

        GetSource.reloadSource(original.id)

        val discovered = GetSource.all().associateBy { it.id }
        assertEquals(setOf(1L, 2L), discovered.keys)
        assertSame(configured, discovered[1])
        assertSame(other, discovered[2])
        assertEquals(1, reloads)
    }

    @Test
    fun `uninstalling a source does not reload it while another extension remains`() {
        val removed = source(1, "Removed extension")
        val other = source(2, "Other extension")
        GetSource.registerSource(removed.id to removed)
        GetSource.registerSource(other.id to other)
        var reloads = 0
        GetSource.reloader = { reloads++ }

        GetSource.unregisterSource(removed.id)

        assertEquals(listOf(other), GetSource.all())
        assertEquals(0, reloads)
    }
}
