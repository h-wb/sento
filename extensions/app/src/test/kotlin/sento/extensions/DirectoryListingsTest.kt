package sento.extensions

import okhttp3.HttpUrl.Companion.toHttpUrl
import eu.kanade.tachiyomi.source.model.SManga
import eu.kanade.tachiyomi.source.online.HttpSource
import kotlinx.coroutines.runBlocking
import okhttp3.Headers
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class DirectoryListingsTest {
    @Test
    fun `adapter uses the source HTTP client independently of its JVM class name`() = runBlocking {
        var requests = 0
        fun source(name: String) = object : HttpSource() {
            override val name = name
            override val lang = "en"
            override val supportsLatest = false
            override val baseUrl = "https://manga.example"
            override fun headersBuilder() = Headers.Builder().add("X-Source-Header", "present")
            override val client = OkHttpClient.Builder().addInterceptor { chain ->
                requests++
                assertEquals("present", chain.request().header("X-Source-Header"))
                assertEquals("/Kingdom", chain.request().url.encodedPath)
                Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1)
                    .code(200).message("OK").body("""<table id="index-table"><tbody>
                        <tr><td><a href="/Kingdom/!Digital">!Digital/</a></td></tr>
                        </tbody></table>""".toResponseBody()).build()
            }.build()
        }
        val manga = SManga.create().apply { url = "/Kingdom"; title = "Kingdom" }
        val listings = directoryListings(source("Madokami"), manga)
        assertEquals(listOf("/Kingdom/!Digital"), listings.map { it.url })
        // A folder keeps its own name: what it holds is for Sento to judge.
        assertEquals(listOf("!Digital"), listings.map { it.title })
        assertEquals(emptyList(), directoryListings(source("Other Source"), manga))
        manga.url = "https://elsewhere.example/Kingdom"
        assertFailsWith<IllegalArgumentException> { directoryListings(source("Madokami"), manga) }
        assertEquals(1, requests)
    }

    @Test
    fun `directory listings include release folders but never files or unrelated paths`() {
        val root = "https://manga.madokami.al/Manga/K/KI/KING/Kingdom".toHttpUrl()
        val links = listOf(
            "/Manga/K/KI/KING/Kingdom/%21%28Digital%29%20%5B1r0n%5D" to "!(Digital) [1r0n]/",
            "/Manga/K/KI/KING/Kingdom/%21ED-Japanese%20Names%20%28Digital%29%20%5B1r0n%2C%20Bingbong%5D" to "!ED-Japanese Names (Digital) [1r0n, Bingbong]/",
            "/Manga/K/KI/KING/Kingdom/%21%28Digital%29%20%5B1r0n%5D/" to "!(Digital) [1r0n]/",
            "/Manga/K/KI/KING/Kingdom/Kingdom%20v01.cbz" to "Kingdom v01.cbz",
            "/Manga/K/KI/KING" to "../",
            "/Manga/K/KI/KING/Kingdom%20Hearts" to "Kingdom Hearts/",
            "/Manga/K/KI/KING/Kingdom/foo/bar" to "not a direct child/",
            "https://example.com/Manga/K/KI/KING/Kingdom/Other" to "Other/",
            "/Manga/K/KI/KING/Kingdom/foo?download=1" to "Download/",
            "/Manga/K/KI/KING/Kingdom/foo#fragment" to "Fragment/",
            "/Manga/K/KI/KING/Kingdom/foo%2Fbar" to "Encoded slash/",
            "/Manga/K/KI/KING/Kingdom/%2E%2E" to "Encoded parent/",
        )
        val html = "<table id='index-table'><tbody>" + links.joinToString("") { (href, name) ->
            "<tr><td><a href='$href'>$name</a></td><td><a href='/unrelated'>Report</a></td></tr>"
        } + "</tbody></table><a href='/Manga/K/KI/KING/Kingdom/NotInListing'>Not a listing/</a>"

        assertEquals(
            links.take(2).map { root.resolve(it.first) to it.second.removeSuffix("/") },
            childDirectories(html, root),
        )
        assertEquals(childDirectories(html, root), childDirectories(html, "$root/".toHttpUrl()))
    }

    @Test
    fun `nested listings stay inside the selected subfolder`() {
        val root = "https://manga.madokami.al/Manga/K/KI/KING/Kingdom/%21Digital".toHttpUrl()
        val html = """<table id="index-table"><tbody>
            <tr><td><a href="/Manga/K/KI/KING/Kingdom">../</a></td></tr>
            <tr><td><a href="/Manga/K/KI/KING/Kingdom/!Digital/!Fixed">!Fixed/</a></td></tr>
            <tr><td><a href="/Manga/K/KI/KING/Kingdom/!Other">!Other/</a></td></tr>
            </tbody></table>"""
        assertEquals(listOf(root.resolve("/Manga/K/KI/KING/Kingdom/!Digital/!Fixed")), childDirectories(html, root).map { it.first })
    }
}
