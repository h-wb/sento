package sento.extensions

import eu.kanade.tachiyomi.network.GET
import eu.kanade.tachiyomi.source.CatalogueSource
import eu.kanade.tachiyomi.source.model.SManga
import eu.kanade.tachiyomi.source.online.HttpSource
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.jsoup.Jsoup
import suwayomi.tachidesk.manga.impl.util.network.await

// Madokami's extension exposes files as chapters, but omits directories.
// Keep its directory protocol here, alongside the extension runtime: Go
// receives ordinary manga keys and never interprets source URLs or HTML.
internal suspend fun directoryListings(source: CatalogueSource, manga: SManga): List<SManga> {
    // Extension jars may obfuscate their JVM class names. The source's
    // catalogue name is the stable identifier for this directory protocol.
    if (source.name != "Madokami") return emptyList()
    val http = source as? HttpSource ?: return emptyList()
    val base = http.baseUrl.toHttpUrl()
    val root = base.resolve(manga.url) ?: return emptyList()
    require(root.scheme == base.scheme && root.host == base.host && root.port == base.port) {
        "Listing is outside the source"
    }
    return http.client.newCall(GET(root.toString(), http.headers)).await().use { response ->
        check(response.isSuccessful) { "Listing source folders: HTTP ${response.code}" }
        childDirectories(response.body.string(), root).map { url ->
            SManga.create().apply {
                this.url = url.encodedPath
                title = manga.title
            }
        }
    }
}

internal fun childDirectories(html: String, root: HttpUrl): List<HttpUrl> {
    val parent = root.pathSegments.filter { it.isNotEmpty() }
    return Jsoup.parse(html, root.toString())
        .select("table#index-table tbody tr td:first-child a[href]")
        .mapNotNull { link ->
            // A directory is marked with a trailing slash in the display
            // name, not necessarily in its href. Never treat archives,
            // parent links, siblings, or another host as nested listings.
            if (!link.text().endsWith("/") || link.text() == "../") return@mapNotNull null
            val url = root.resolve(link.attr("href")) ?: return@mapNotNull null
            val path = url.pathSegments.filter { it.isNotEmpty() }
            if (url.scheme != root.scheme || url.host != root.host || url.port != root.port ||
                url.query != null || url.fragment != null ||
                path.size != parent.size + 1 || path.take(parent.size) != parent ||
                path.last().contains('/') || path.last().contains('\\')
            ) return@mapNotNull null
            url.newBuilder().encodedPath(url.encodedPath.trimEnd('/')).build()
        }.distinct()
}
