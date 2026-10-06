package sento.host

/*
 * Install/uninstall logic adapted from Suwayomi-Server's Extension.kt
 * (Copyright Contributors to the Suwayomi project, MPL-2.0), with the
 * database replaced by a JSON index next to the jars: the filesystem is
 * the only state.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/. */

import android.content.pm.PackageInfo
import eu.kanade.tachiyomi.network.GET
import eu.kanade.tachiyomi.network.NetworkHelper
import eu.kanade.tachiyomi.source.CatalogueSource
import eu.kanade.tachiyomi.source.Source
import eu.kanade.tachiyomi.source.SourceFactory
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromByteArray
import kotlinx.serialization.json.Json
import kotlinx.serialization.protobuf.ProtoBuf
import okhttp3.CacheControl
import okio.buffer
import okio.gzip
import okio.sink
import suwayomi.tachidesk.manga.impl.extension.github.NetworkExtensionStore
import suwayomi.tachidesk.manga.impl.extension.github.NetworkLegacyExtensionRepo
import suwayomi.tachidesk.manga.impl.extension.github.toExtensionInfos
import suwayomi.tachidesk.manga.impl.util.PackageTools
import suwayomi.tachidesk.manga.impl.util.PackageTools.EXTENSION_FEATURE
import suwayomi.tachidesk.manga.impl.util.PackageTools.LIB_VERSION_MAX
import suwayomi.tachidesk.manga.impl.util.PackageTools.LIB_VERSION_MIN
import suwayomi.tachidesk.manga.impl.util.PackageTools.METADATA_NSFW
import suwayomi.tachidesk.manga.impl.util.PackageTools.METADATA_SOURCE_CLASS
import suwayomi.tachidesk.manga.impl.util.PackageTools.dex2jar
import suwayomi.tachidesk.manga.impl.util.PackageTools.getPackageInfo
import suwayomi.tachidesk.manga.impl.util.PackageTools.loadExtensionSources
import suwayomi.tachidesk.manga.impl.util.network.await
import suwayomi.tachidesk.manga.impl.util.source.GetSource
import suwayomi.tachidesk.manga.model.dataclass.ExtensionInfo
import suwayomi.tachidesk.server.ApplicationDirs
import uy.kohesive.injekt.injectLazy
import java.io.File
import java.io.FileOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipInputStream
import java.util.zip.ZipOutputStream
import kotlin.io.path.Path
import kotlin.io.path.absolutePathString

@Serializable
data class InstalledExtension(
    val pkgName: String,
    val name: String,
    val apkName: String,
    val className: String,
    val versionName: String,
    val versionCode: Long,
    val lang: String,
    val isNsfw: Boolean,
    val repo: String,
)

object ExtensionManager {
    private val logger = KotlinLogging.logger {}
    private val applicationDirs: ApplicationDirs by injectLazy()
    private val network: NetworkHelper by injectLazy()
    private val json = Json { ignoreUnknownKeys = true; prettyPrint = true }

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

    private val indexFile get() = File(applicationDirs.extensionsRoot, "installed.json")

    @Volatile
    private var installed: Map<String, InstalledExtension> = emptyMap()

    val installedExtensions: Collection<InstalledExtension> get() = installed.values

    @Synchronized
    fun loadInstalled() {
        installed =
            runCatching {
                json
                    .decodeFromString<List<InstalledExtension>>(indexFile.readText())
                    .associateBy { it.pkgName }
            }.getOrDefault(emptyMap())
        registerAll()
        GetSource.reloader = ::registerAll
    }

    private fun registerAll() {
        installed.values.forEach { ext ->
            runCatching { registerSources(jarPath(ext.apkName), ext.className) }
                .onFailure { logger.error(it) { "loading ${ext.pkgName} failed" } }
        }
    }

    private fun persist(next: Map<String, InstalledExtension>) {
        installed = next
        indexFile.parentFile.mkdirs()
        indexFile.writeText(json.encodeToString(installed.values.sortedBy { it.pkgName }))
    }

    private fun jarPath(apkName: String) =
        "${applicationDirs.extensionsRoot}/${apkName.substringBefore(".apk")}.jar"

    // The index no longer carries a file name, only a URL. Fall back to
    // the package name so a URL without a usable last segment still
    // produces a stable, collision-free jar on disk.
    private fun apkNameOf(ext: ExtensionInfo): String {
        val fromUrl = ext.apkUrl.substringAfterLast('/').substringBefore('?')
        return if (fromUrl.endsWith(".apk")) fromUrl else "${ext.pkgName}.apk"
    }

    // PackageTools keys its loader map by absolute path.
    private fun loaderKey(jarFilePath: String) = Path(jarFilePath).absolutePathString()

    // Repo list: the env-provided extras plus whatever Sento pushed
    // (volatile — Sento reconciles it like extensions and preferences).
    @Volatile
    private var pushedRepos: List<String> = emptyList()

    fun repos(): List<String> = (HostConfig.repos + pushedRepos).distinct()

    fun setRepos(urls: List<String>) {
        pushedRepos = urls
        availableCache = null
    }

    // A repo index is ~1500 entries fetched from a raw URL; cache it
    // briefly — Sento's reconcile loop polls regularly.
    @Volatile
    private var availableCache: Pair<Long, List<ExtensionInfo>>? = null

    suspend fun available(): List<ExtensionInfo> {
        availableCache?.let { (fetchedAt, list) ->
            if (System.currentTimeMillis() - fetchedAt < 5 * 60_000) return list
        }
        val list =
            repos().flatMap { repo ->
                runCatching { findExtensions(repo) }
                    .onFailure { logger.warn(it) { "extension repo $repo failed" } }
                    .getOrDefault(emptyList())
            }
        if (list.isNotEmpty()) availableCache = System.currentTimeMillis() to list
        return list
    }

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
    private suspend fun findExtensions(indexUrl: String): List<ExtensionInfo> {
        val infos = resolveIndex(indexUrl, indexUrl, hops = 0)

        // Extensions built against a lib version this runtime cannot load
        // would fail at install; drop them from the catalogue instead.
        return infos.filter { info ->
            val lib = info.extensionLib.toDoubleOrNull()
            if (lib == null) {
                false
            } else {
                lib in LIB_VERSION_MIN..LIB_VERSION_MAX
            }
        }
    }

    // configuredUrl is what the user entered and stays the store identity
    // however many documents the resolution walks through.
    @OptIn(ExperimentalSerializationApi::class)
    private suspend fun resolveIndex(
        configuredUrl: String,
        url: String,
        hops: Int,
    ): List<ExtensionInfo> {
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
    ): List<ExtensionInfo> {
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
        return list.toExtensionInfos(store)
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

    suspend fun install(pkgName: String): InstalledExtension {
        val ext =
            available().filter { it.pkgName == pkgName }.maxByOrNull { it.versionCode }
                ?: throw IllegalArgumentException("extension $pkgName not found in any repo")
        return install(ext)
    }

    @Suppress("ktlint")
    suspend fun install(ext: ExtensionInfo): InstalledExtension {
        logger.info { "installing ${ext.pkgName} ${ext.versionName}" }
        installed[ext.pkgName]?.let { uninstall(it.pkgName) }

        File(applicationDirs.extensionsRoot).mkdirs()
        // Indexes state the APK URL outright; the file name is only
        // used to key the jar on disk.
        val apkName = apkNameOf(ext)
        val apkPath = "${applicationDirs.extensionsRoot}/$apkName"
        downloadFile(ext.apkUrl, apkPath)

        val jarFilePath = jarPath(apkName)
        val packageInfo = getPackageInfo(Path(apkPath))
        try {
            check(packageInfo.reqFeatures.orEmpty().any { it.name == EXTENSION_FEATURE }) {
                "$apkPath is not a Tachiyomi extension"
            }
            val libVersion = packageInfo.versionName.substringBeforeLast('.').toDouble()
            check(libVersion in LIB_VERSION_MIN..LIB_VERSION_MAX) {
                "lib version $libVersion outside supported range $LIB_VERSION_MIN..$LIB_VERSION_MAX"
            }
            val className = sourceClassNames(packageInfo).joinToString(";")
            check(className.isNotEmpty()) { "$apkPath declares no $METADATA_SOURCE_CLASS" }
            val isNsfw = packageInfo.applicationInfo.metaData.getString(METADATA_NSFW) == "1"

            // Some repos publish the converted jar next to the APK. Prefer it:
            // dex2jar's output for newer extensions can fail JVM bytecode
            // verification (bad stackmap frames in the suspend getMangaUpdate),
            // and a publisher-built jar sidesteps the conversion entirely.
            // The APK is still needed for its manifest metadata.
            val jarUrl = ext.jarUrl
            if (jarUrl != null) {
                downloadFile(jarUrl, jarFilePath)
            } else {
                dex2jar(Path(apkPath), Path(jarFilePath))
            }
            extractAssetsFromApk(apkPath, jarFilePath)

            registerSources(jarFilePath, className)

            val record =
                InstalledExtension(
                    pkgName = packageInfo.packageName,
                    name = ext.name,
                    apkName = apkName,
                    className = className,
                    versionName = packageInfo.versionName,
                    versionCode = ext.versionCode,
                    lang = ext.lang,
                    isNsfw = isNsfw,
                    repo = ext.storeIndexUrl,
                )
            persist(installed + (record.pkgName to record))
            return record
        } catch (e: Throwable) {
            PackageTools.jarLoaderMap.remove(loaderKey(jarFilePath))?.close()
            File(jarFilePath).delete()
            throw e
        } finally {
            File(apkPath).delete()
        }
    }

    @Synchronized
    fun uninstall(pkgName: String) {
        val ext = installed[pkgName] ?: return
        val jar = jarPath(ext.apkName)
        PackageTools.jarLoaderMap.remove(loaderKey(jar))?.close()
        runCatching { sourcesOf(jar, ext.className).forEach { GetSource.unregisterSource(it.id) } }
        File(jar).delete()
        persist(installed - pkgName)
    }

    // sourcesFor lists the sources one installed extension provides (the
    // Add Indexer flow picks among them).
    fun sourcesFor(pkgName: String): List<CatalogueSource> {
        val ext = installed[pkgName] ?: return emptyList()
        return runCatching { sourcesOf(jarPath(ext.apkName), ext.className) }
            .getOrDefault(emptyList())
    }

    private fun registerSources(
        jarFilePath: String,
        className: String,
    ) {
        sourcesOf(jarFilePath, className).forEach {
            GetSource.registerSource(it.id to it)
        }
    }

    // An APK may declare several source classes, ';'-separated; a name is
    // package-relative only when it starts with a dot, otherwise it is
    // already fully qualified (Mihon's convention — concatenating
    // unconditionally yields class names that do not exist).
    private fun sourceClassNames(packageInfo: PackageInfo): List<String> =
        packageInfo.applicationInfo.metaData
            .getString(METADATA_SOURCE_CLASS)
            .orEmpty()
            .split(';')
            .mapNotNull { it.trim().ifEmpty { null } }
            .map { if (it.startsWith(".")) packageInfo.packageName + it else it }

    private fun sourcesOf(
        jarFilePath: String,
        className: String,
    ): List<CatalogueSource> =
        className.split(';').filter { it.isNotEmpty() }.flatMap { name ->
            // Suspending since Suwayomi v2.4: it takes a per-jar mutex that
            // their extension updater can hold across an update. Nothing in
            // this host holds it (installs are @Synchronized here), so the
            // bridge never actually waits.
            when (val instance = runBlocking { loadExtensionSources(Path(jarFilePath), name) }) {
                is Source -> listOf(instance)
                is SourceFactory -> instance.createSources()
                else -> throw RuntimeException("unknown source class type ${instance.javaClass}")
            }.map { it as CatalogueSource }
        }

    private suspend fun downloadFile(
        url: String,
        savePath: String,
    ) {
        val response = network.client.newCall(GET(url, cache = CacheControl.FORCE_NETWORK)).await()
        check(response.isSuccessful) { "download of $url returned HTTP ${response.code}" }
        File(savePath).sink().buffer().use { sink ->
            response.body.source().use { source ->
                sink.writeAll(source)
                sink.flush()
            }
        }
    }

    // Verbatim from Suwayomi's Extension.kt: extensions may carry assets
    // (fonts, js) that must live next to the classes in the jar.
    private fun extractAssetsFromApk(
        apkPath: String,
        jarPath: String,
    ) {
        val apkFile = File(apkPath)
        val jarFile = File(jarPath)

        val assetsFolder = File("${apkFile.parent}/${apkFile.nameWithoutExtension}_assets")
        assetsFolder.mkdir()
        ZipInputStream(apkFile.inputStream()).use { zipInputStream ->
            var zipEntry = zipInputStream.nextEntry
            while (zipEntry != null) {
                if (zipEntry.name.startsWith("assets/") && !zipEntry.isDirectory) {
                    val assetFile = File(assetsFolder, zipEntry.name)
                    assetFile.parentFile.mkdirs()
                    FileOutputStream(assetFile).use { outputStream ->
                        zipInputStream.copyTo(outputStream)
                    }
                }
                zipEntry = zipInputStream.nextEntry
            }
        }

        val tempJarFile = File("${jarFile.parent}/${jarFile.nameWithoutExtension}_temp.jar")
        ZipInputStream(jarFile.inputStream()).use { jarZipInputStream ->
            ZipOutputStream(FileOutputStream(tempJarFile)).use { jarZipOutputStream ->
                // A publisher-built jar may already carry the APK's assets
                // (dex2jar output never does). Its copy wins: a second
                // entry of the same name is a ZipException, which used to
                // fail the whole install.
                val written = mutableSetOf<String>()
                var zipEntry = jarZipInputStream.nextEntry
                while (zipEntry != null) {
                    if (!zipEntry.name.startsWith("META-INF/") && written.add(zipEntry.name)) {
                        jarZipOutputStream.putNextEntry(ZipEntry(zipEntry.name))
                        jarZipInputStream.copyTo(jarZipOutputStream)
                    }
                    zipEntry = jarZipInputStream.nextEntry
                }
                assetsFolder.walkTopDown().forEach { file ->
                    val name = file.relativeTo(assetsFolder).toString().replace("\\", "/")
                    if (file.isFile && written.add(name)) {
                        jarZipOutputStream.putNextEntry(
                            ZipEntry(name),
                        )
                        file.inputStream().use { inputStream ->
                            inputStream.copyTo(jarZipOutputStream)
                        }
                        jarZipOutputStream.closeEntry()
                    }
                }
            }
        }

        jarFile.delete()
        tempJarFile.renameTo(jarFile)

        assetsFolder.deleteRecursively()
    }
}
