package sento.host

// HTTP API mirroring Sento's internal/sourcehost.Host contract: the host
// is a stateless protocol adapter — sources, manga search, chapter lists,
// page refs, page bytes. Keys are JSON envelopes of what the extension
// API needs back (url + title/name), opaque to the caller.

import eu.kanade.tachiyomi.source.CatalogueSource
import eu.kanade.tachiyomi.source.model.FilterList
import eu.kanade.tachiyomi.source.model.Page
import eu.kanade.tachiyomi.source.model.SChapter
import eu.kanade.tachiyomi.source.model.SManga
import eu.kanade.tachiyomi.source.online.HttpSource
import eu.kanade.tachiyomi.util.chapter.ChapterRecognition
import io.javalin.Javalin
import io.javalin.http.Context
import io.javalin.http.HttpStatus
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import suwayomi.tachidesk.manga.model.dataclass.ContentWarning

private val json = Json { ignoreUnknownKeys = true }

@Serializable
data class SourceDto(
    val id: String,
    val name: String,
    val lang: String,
    val nsfw: Boolean,
)

@Serializable
data class MangaDto(
    val key: String,
    val title: String,
)

@Serializable
data class ChapterDto(
    val key: String,
    val name: String,
    val number: Double,
    val scanlator: String? = null,
    val webUrl: String? = null,
)

@Serializable
data class ExtensionDto(
    val pkgName: String,
    val name: String,
    val lang: String,
    val nsfw: Boolean,
    val installedVersion: String? = null,
    val availableVersion: String? = null,
    val hasUpdate: Boolean = false,
)

@Serializable
private data class MangaKey(val url: String, val title: String)

@Serializable
private data class ChapterKey(val url: String, val name: String)

@Serializable
private data class PageRef(val index: Int, val url: String, val imageUrl: String? = null)

@Serializable
data class RepoListDto(val urls: List<String>)

@Serializable
private data class ErrorDto(val error: String)

private inline fun <reified T> Context.respond(value: T) {
    contentType("application/json").result(json.encodeToString(value))
}

private fun Context.sourceOrNull(): CatalogueSource? {
    val id = pathParam("sid").toLongOrNull() ?: return null
    return suwayomi.tachidesk.manga.impl.util.source.GetSource.getSourceOrNull(id)
}

fun buildApi(): Javalin =
    Javalin.create { config ->
        config.startup.showJavalinBanner = false
        config.routes.defineEndpoints()
    }

private fun io.javalin.config.RoutesConfig.defineEndpoints() {
    exception(Exception::class.java) { e, ctx ->
        ctx.status(HttpStatus.BAD_GATEWAY)
        ctx.respond(ErrorDto(e.message ?: e.javaClass.simpleName))
    }

    get("/health") { ctx -> ctx.result("ok") }

    get("/api/v1/sources") { ctx ->
        ctx.respond(
            suwayomi.tachidesk.manga.impl.util.source.GetSource
                .all()
                .map { SourceDto(it.id.toString(), it.name, it.lang, isNsfw(it)) },
        )
    }

    get("/api/v1/sources/{sid}/search") { ctx ->
        val source = ctx.sourceOrNull() ?: return@get ctx.notFound("unknown source")
        val query = ctx.queryParam("q") ?: return@get ctx.badRequest("missing query parameter q")
        val page = runBlocking { source.getSearchManga(1, query, FilterList()) }
        ctx.respond(
            page.mangas.map {
                MangaDto(key = json.encodeToString(MangaKey(it.url, it.title)), title = it.title)
            },
        )
    }

    get("/api/v1/sources/{sid}/chapters") { ctx ->
        val source = ctx.sourceOrNull() ?: return@get ctx.notFound("unknown source")
        val keyRaw = ctx.queryParam("manga") ?: return@get ctx.badRequest("missing query parameter manga")
        val key = json.decodeFromString<MangaKey>(keyRaw)
        val manga =
            SManga.create().apply {
                url = key.url
                title = key.title
            }
        val chapters =
            runBlocking {
                source
                    .getMangaUpdate(manga, emptyList(), fetchDetails = false, fetchChapters = true)
                    .chapters
            }
        ctx.respond(
            chapters.map {
                // Same corroboration Suwayomi applies: sources often leave
                // chapter_number unset and the name carries the number.
                val number =
                    ChapterRecognition.parseChapterNumber(manga.title, it.name, it.chapter_number.toDouble())
                ChapterDto(
                    key = json.encodeToString(ChapterKey(it.url, it.name)),
                    name = it.name,
                    number = number,
                    scanlator = it.scanlator,
                    // No network: the default builds the page-list request URL.
                    webUrl = (source as? HttpSource)?.let { hs -> runCatching { hs.getChapterUrl(it) }.getOrNull() },
                )
            },
        )
    }

    get("/api/v1/sources/{sid}/pages") { ctx ->
        val source = ctx.sourceOrNull() ?: return@get ctx.notFound("unknown source")
        val keyRaw = ctx.queryParam("chapter") ?: return@get ctx.badRequest("missing query parameter chapter")
        val key = json.decodeFromString<ChapterKey>(keyRaw)
        val chapter =
            SChapter.create().apply {
                url = key.url
                name = key.name
            }
        val pages = runBlocking { source.getPageList(chapter) }
        ctx.respond(
            pages.map { json.encodeToString(PageRef(it.index, it.url, it.imageUrl)) },
        )
    }

    get("/api/v1/sources/{sid}/page") { ctx ->
        val source = ctx.sourceOrNull() ?: return@get ctx.notFound("unknown source")
        val http = source as? HttpSource ?: return@get ctx.badRequest("source is not an HttpSource")
        val ref = ctx.queryParam("ref") ?: return@get ctx.badRequest("missing query parameter ref")
        val parsed = json.decodeFromString<PageRef>(ref)
        val page = Page(parsed.index, parsed.url, parsed.imageUrl)
        val response =
            runBlocking {
                if (page.imageUrl.isNullOrEmpty()) {
                    page.imageUrl = http.getImageUrl(page)
                }
                http.getImage(page)
            }
        ctx.contentType(response.header("Content-Type") ?: "image/jpeg")
        response.body.byteStream().use { ctx.result(it.readBytes()) }
    }

    // --- source preferences (ConfigurableSource) ---

    get("/api/v1/sources/{sid}/preferences") { ctx ->
        val source = ctx.sourceOrNull() ?: return@get ctx.notFound("unknown source")
        ctx.respond(SourcePrefs.list(source))
    }

    post("/api/v1/sources/{sid}/preferences") { ctx ->
        val source = ctx.sourceOrNull() ?: return@post ctx.notFound("unknown source")
        SourcePrefs.set(source, json.decodeFromString<SetPreferenceBody>(ctx.body()))
        ctx.status(HttpStatus.NO_CONTENT)
    }

    // --- extension repos (volatile; Sento reconciles its stored list) ---

    get("/api/v1/repos") { ctx ->
        ctx.respond(RepoListDto(ExtensionManager.repos()))
    }

    put("/api/v1/repos") { ctx ->
        ExtensionManager.setRepos(json.decodeFromString<RepoListDto>(ctx.body()).urls)
        ctx.status(HttpStatus.NO_CONTENT)
    }

    // --- extension management ---

    get("/api/v1/extensions") { ctx ->
        val installed = ExtensionManager.installedExtensions.associateBy { it.pkgName }
        val available = runBlocking { ExtensionManager.available() }
        val merged = LinkedHashMap<String, ExtensionDto>()
        for (ext in available) {
            val inst = installed[ext.pkgName]
            val prev = merged[ext.pkgName]
            if (prev != null && (prev.availableVersion ?: "") >= ext.versionName) continue
            merged[ext.pkgName] =
                ExtensionDto(
                    pkgName = ext.pkgName,
                    name = ext.name,
                    lang = ext.lang,
                    nsfw = ext.contentWarning != ContentWarning.SAFE,
                    installedVersion = inst?.versionName,
                    availableVersion = ext.versionName,
                    hasUpdate = inst != null && ext.versionCode > inst.versionCode,
                )
        }
        for ((pkg, inst) in installed) {
            merged.putIfAbsent(
                pkg,
                ExtensionDto(
                    pkgName = pkg,
                    name = inst.name,
                    lang = inst.lang,
                    nsfw = inst.isNsfw,
                    installedVersion = inst.versionName,
                ),
            )
        }
        ctx.respond(merged.values.sortedBy { it.pkgName }.toList())
    }

    get("/api/v1/extensions/{pkg}/sources") { ctx ->
        ctx.respond(
            ExtensionManager.sourcesFor(ctx.pathParam("pkg")).map {
                SourceDto(it.id.toString(), it.name, it.lang, isNsfw(it))
            },
        )
    }

    post("/api/v1/extensions/{pkg}") { ctx ->
        val record = runBlocking { ExtensionManager.install(ctx.pathParam("pkg")) }
        ctx.status(HttpStatus.CREATED)
        ctx.respond(
            ExtensionDto(
                pkgName = record.pkgName,
                name = record.name,
                lang = record.lang,
                nsfw = record.isNsfw,
                installedVersion = record.versionName,
            ),
        )
    }

    delete("/api/v1/extensions/{pkg}") { ctx ->
        ExtensionManager.uninstall(ctx.pathParam("pkg"))
        ctx.status(HttpStatus.NO_CONTENT)
    }
}

private fun Context.notFound(msg: String) {
    status(HttpStatus.NOT_FOUND)
    respond(ErrorDto(msg))
}

private fun Context.badRequest(msg: String) {
    status(HttpStatus.BAD_REQUEST)
    respond(ErrorDto(msg))
}

private fun isNsfw(source: CatalogueSource): Boolean =
    ExtensionManager.installedExtensions.any { ext ->
        ext.isNsfw && source.javaClass.name.startsWith(ext.pkgName)
    }
