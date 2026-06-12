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

import eu.kanade.tachiyomi.network.GET
import eu.kanade.tachiyomi.network.NetworkHelper
import eu.kanade.tachiyomi.source.CatalogueSource
import eu.kanade.tachiyomi.source.Source
import eu.kanade.tachiyomi.source.SourceFactory
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.CacheControl
import okio.buffer
import okio.sink
import suwayomi.tachidesk.manga.impl.extension.github.ExtensionGithubApi
import suwayomi.tachidesk.manga.impl.extension.github.OnlineExtension
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
import suwayomi.tachidesk.manga.impl.util.source.GetCatalogueSource
import suwayomi.tachidesk.server.ApplicationDirs
import uy.kohesive.injekt.injectLazy
import java.io.File
import java.io.FileOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipInputStream
import java.util.zip.ZipOutputStream

@Serializable
data class InstalledExtension(
    val pkgName: String,
    val name: String,
    val apkName: String,
    val className: String,
    val versionName: String,
    val versionCode: Int,
    val lang: String,
    val isNsfw: Boolean,
    val repo: String,
)

object ExtensionManager {
    private val logger = KotlinLogging.logger {}
    private val applicationDirs: ApplicationDirs by injectLazy()
    private val network: NetworkHelper by injectLazy()
    private val json = Json { ignoreUnknownKeys = true; prettyPrint = true }

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
        GetCatalogueSource.reloader = ::registerAll
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
    private var availableCache: Pair<Long, List<OnlineExtension>>? = null

    suspend fun available(): List<OnlineExtension> {
        availableCache?.let { (fetchedAt, list) ->
            if (System.currentTimeMillis() - fetchedAt < 5 * 60_000) return list
        }
        val list =
            repos().flatMap { repo ->
                runCatching { ExtensionGithubApi.findExtensions(repo) }
                    .onFailure { logger.warn(it) { "extension repo $repo failed" } }
                    .getOrDefault(emptyList())
            }
        if (list.isNotEmpty()) availableCache = System.currentTimeMillis() to list
        return list
    }

    suspend fun install(pkgName: String): InstalledExtension {
        val ext =
            available().filter { it.pkgName == pkgName }.maxByOrNull { it.versionCode }
                ?: throw IllegalArgumentException("extension $pkgName not found in any repo")
        return install(ext)
    }

    @Suppress("ktlint")
    suspend fun install(ext: OnlineExtension): InstalledExtension {
        logger.info { "installing ${ext.pkgName} ${ext.versionName}" }
        installed[ext.pkgName]?.let { uninstall(it.pkgName) }

        File(applicationDirs.extensionsRoot).mkdirs()
        val apkPath = "${applicationDirs.extensionsRoot}/${ext.apkName}"
        downloadFile(ExtensionGithubApi.getApkUrl(ext.repo, ext.apkName), apkPath)

        val jarFilePath = jarPath(ext.apkName)
        val packageInfo = getPackageInfo(apkPath)
        try {
            check(packageInfo.reqFeatures.orEmpty().any { it.name == EXTENSION_FEATURE }) {
                "$apkPath is not a Tachiyomi extension"
            }
            val libVersion = packageInfo.versionName.substringBeforeLast('.').toDouble()
            check(libVersion in LIB_VERSION_MIN..LIB_VERSION_MAX) {
                "lib version $libVersion outside supported range $LIB_VERSION_MIN..$LIB_VERSION_MAX"
            }
            val className =
                packageInfo.packageName +
                    packageInfo.applicationInfo.metaData.getString(METADATA_SOURCE_CLASS)
            val isNsfw = packageInfo.applicationInfo.metaData.getString(METADATA_NSFW) == "1"

            dex2jar(apkPath, jarFilePath, ext.apkName.substringBefore(".apk"))
            extractAssetsFromApk(apkPath, jarFilePath)

            registerSources(jarFilePath, className)

            val record =
                InstalledExtension(
                    pkgName = packageInfo.packageName,
                    name = ext.name,
                    apkName = ext.apkName,
                    className = className,
                    versionName = packageInfo.versionName,
                    versionCode = ext.versionCode,
                    lang = ext.lang,
                    isNsfw = isNsfw,
                    repo = ext.repo,
                )
            persist(installed + (record.pkgName to record))
            return record
        } catch (e: Throwable) {
            PackageTools.jarLoaderMap.remove(jarFilePath)?.close()
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
        PackageTools.jarLoaderMap.remove(jar)?.close()
        runCatching { sourcesOf(jar, ext.className).forEach { GetCatalogueSource.unregisterCatalogueSource(it.id) } }
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
            GetCatalogueSource.registerCatalogueSource(it.id to it)
        }
    }

    private fun sourcesOf(
        jarFilePath: String,
        className: String,
    ): List<CatalogueSource> =
        when (val instance = loadExtensionSources(jarFilePath, className)) {
            is Source -> listOf(instance)
            is SourceFactory -> instance.createSources()
            else -> throw RuntimeException("unknown source class type ${instance.javaClass}")
        }.map { it as CatalogueSource }

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
                var zipEntry = jarZipInputStream.nextEntry
                while (zipEntry != null) {
                    if (!zipEntry.name.startsWith("META-INF/")) {
                        jarZipOutputStream.putNextEntry(ZipEntry(zipEntry.name))
                        jarZipInputStream.copyTo(jarZipOutputStream)
                    }
                    zipEntry = jarZipInputStream.nextEntry
                }
                assetsFolder.walkTopDown().forEach { file ->
                    if (file.isFile) {
                        jarZipOutputStream.putNextEntry(
                            ZipEntry(file.relativeTo(assetsFolder).toString().replace("\\", "/")),
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
