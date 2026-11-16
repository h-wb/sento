package sento.extensions

import eu.kanade.tachiyomi.network.GET
import eu.kanade.tachiyomi.network.NetworkHelper
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.decodeFromByteArray
import kotlinx.serialization.json.Json
import kotlinx.serialization.protobuf.ProtoBuf
import okhttp3.CacheControl
import okio.buffer
import okio.gzip
import suwayomi.tachidesk.manga.impl.extension.github.NetworkExtensionStore
import suwayomi.tachidesk.manga.impl.extension.github.NetworkLegacyExtensionRepo
import suwayomi.tachidesk.manga.impl.extension.github.toExtensionInfos
import suwayomi.tachidesk.manga.impl.util.PackageTools.LIB_VERSION_MAX
import suwayomi.tachidesk.manga.impl.util.PackageTools.LIB_VERSION_MIN
import suwayomi.tachidesk.manga.impl.util.network.await
import suwayomi.tachidesk.manga.model.dataclass.ExtensionInfo
import uy.kohesive.injekt.injectLazy

internal object RepoIndex {
    private val network: NetworkHelper by injectLazy()

    // Repo indexes are third-party JSON and need a looser reader than our
    // own state file: `explicitNulls` so an omitted nullable field decodes
    // as null (upstream declares several without defaults, and real indexes
    // leave them out), `isLenient` because publishers quote their numbers.
    @Suppress("ktlint")
    private val indexJson = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        isLenient = true
    }

    // The v2 index format is protobuf (same field numbers as the JSON one).
    @OptIn(ExperimentalSerializationApi::class)
    private val indexProto = ProtoBuf

    private const val OLD_INDEX = "index.min.json"
    private const val REPO_POINTER = "repo.json"
    private const val GZIP_MAGIC = 0x1f8b
    private const val MAX_INDEX_HOPS = 3

    // Catalogue is one repo's resolved index: the store's own name and
    // its extensions.
    data class Catalogue(
        val name: String,
        val extensions: List<ExtensionInfo>,
    )

    // A repo publishes a store document — protobuf (usually gzipped) or
    // JSON — carrying its extension list inline or behind
    // extensionListUrl, with an explicit apkUrl per extension. The URL a
    // user configured may not be that document itself: repos that predate
    // the format keep their old index.min.json address alive and announce
    // the store through a repo.json next to it ("index_v2"), so those two
    // are followed to the store. (This mirrors Suwayomi's
    // ExtensionStoreService, whose own fetching lives behind its database
    // layer.)
    //
    // The old flat list itself is not read. Repos that moved on serve only
    // placeholder entries there ("Outdated App"), so taking it at face
    // value yields a catalogue of nothing; a repo that publishes nothing
    // else is reported as unsupported instead.
    suspend fun findExtensions(indexUrl: String): Catalogue {
        val catalogue = resolveIndex(indexUrl, indexUrl, hops = 0)

        // Extensions built against a lib version this runtime cannot load
        // would fail at install; drop them from the catalogue instead.
        return catalogue.copy(
            extensions =
                catalogue.extensions.filter { info ->
                    val lib = info.extensionLib.toDoubleOrNull()
                    if (lib == null) {
                        false
                    } else {
                        lib in LIB_VERSION_MIN..LIB_VERSION_MAX
                    }
                },
        )
    }

    // configuredUrl is what the user entered and stays the store identity
    // however many documents the resolution walks through.
    @OptIn(ExperimentalSerializationApi::class)
    private suspend fun resolveIndex(
        configuredUrl: String,
        url: String,
        hops: Int,
    ): Catalogue {
        check(hops <= MAX_INDEX_HOPS) { "extension repo $configuredUrl redirects through too many index documents" }
        val body = fetchBytes(url)
        return when (body.firstOrNull { !it.toInt().toChar().isWhitespace() }) {
            // The old flat list: only good for finding the repo.json beside it.
            '['.code.toByte() -> {
                check(url.endsWith("/$OLD_INDEX")) { "extension repo $configuredUrl is not a store index" }
                resolveIndex(configuredUrl, url.removeSuffix(OLD_INDEX) + REPO_POINTER, hops + 1)
            }

            '{'.code.toByte() -> {
                val text = body.decodeToString()
                val pointer = runCatching { indexJson.decodeFromString<NetworkLegacyExtensionRepo>(text) }.getOrNull()
                if (pointer == null) {
                    storeExtensions(configuredUrl, indexJson.decodeFromString<NetworkExtensionStore>(text))
                } else {
                    val store =
                        checkNotNull(pointer.indexV2) {
                            "extension repo $configuredUrl only publishes the old index format, which is no longer supported"
                        }
                    resolveIndex(configuredUrl, store, hops + 1)
                }
            }

            else -> storeExtensions(configuredUrl, indexProto.decodeFromByteArray<NetworkExtensionStore>(body))
        }
    }

    @OptIn(ExperimentalSerializationApi::class)
    private suspend fun storeExtensions(
        configuredUrl: String,
        network: NetworkExtensionStore,
    ): Catalogue {
        val store = network.toExtensionStore(configuredUrl)
        // The list is either inline or behind a second URL, in either
        // encoding.
        val list =
            network.extensionList
                ?: network.extensionListUrl?.let { listUrl ->
                    val body = fetchBytes(listUrl)
                    if (body.firstOrNull() == '{'.code.toByte()) {
                        indexJson.decodeFromString<NetworkExtensionStore.ExtensionList>(body.decodeToString())
                    } else {
                        indexProto.decodeFromByteArray<NetworkExtensionStore.ExtensionList>(body)
                    }
                }
                ?: NetworkExtensionStore.ExtensionList(emptyList())
        return Catalogue(network.name, list.toExtensionInfos(store))
    }

    // fetchBytes returns a document's bytes, gunzipped when it is served
    // compressed as a file (index.pb is; that is not HTTP content-encoding,
    // which okhttp already undoes).
    private suspend fun fetchBytes(url: String): ByteArray {
        val response = network.client.newCall(GET(url, cache = CacheControl.FORCE_NETWORK)).await()
        response.use {
            check(it.isSuccessful) { "fetch of $url returned HTTP ${it.code}" }
            val source = it.body.source()
            val gzipped = source.peek().use { head -> runCatching { head.readShort().toInt() == GZIP_MAGIC }.getOrDefault(false) }
            return if (gzipped) source.gzip().buffer().readByteArray() else source.readByteArray()
        }
    }
}
