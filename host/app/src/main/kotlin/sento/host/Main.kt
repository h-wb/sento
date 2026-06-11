package sento.host

import android.app.Application
import eu.kanade.tachiyomi.network.JavaScriptEngine
import eu.kanade.tachiyomi.network.NetworkHelper
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.json.Json
import org.koin.core.context.startKoin
import org.koin.dsl.module
import suwayomi.tachidesk.server.ApplicationDirs
import xyz.nulldev.androidcompat.AndroidCompat
import xyz.nulldev.androidcompat.AndroidCompatInitializer
import xyz.nulldev.androidcompat.androidCompatModule
import xyz.nulldev.androidcompat.webkit.KcefWebViewProvider
import xyz.nulldev.ts.config.GlobalConfigManager
import xyz.nulldev.ts.config.configManagerModule
import java.io.File

class HostApp : Application()

private val logger = KotlinLogging.logger {}

fun main() {
    // The AndroidCompat config tree roots itself here (shared_prefs,
    // fake app dirs); extensions and their jars live under it too.
    System.setProperty("suwayomi.tachidesk.config.server.rootDir", HostConfig.dataRoot)

    val applicationDirs = ApplicationDirs()
    listOf(applicationDirs.dataRoot, applicationDirs.extensionsRoot).forEach { File(it).mkdirs() }

    val app = HostApp()
    startKoin {
        modules(
            configManagerModule(),
            androidCompatModule(),
            module {
                single<Application> { app }
                single { applicationDirs }
                single { NetworkHelper(get<xyz.nulldev.androidcompat.androidimpl.CustomContext>()) }
                single { JavaScriptEngine(get<xyz.nulldev.androidcompat.androidimpl.CustomContext>()) }
                single { Json { ignoreUnknownKeys = true } }
                // WebView is best-effort: no KCEF browser is provisioned.
                single<KcefWebViewProvider.InitBrowserHandler> {
                    object : KcefWebViewProvider.InitBrowserHandler {
                        override fun init(provider: KcefWebViewProvider) = Unit
                    }
                }
            },
        )
    }
    // Force config-module registration before anything touches Android APIs.
    GlobalConfigManager.config

    AndroidCompatInitializer().init()
    AndroidCompat().startApp(app)

    ExtensionManager.loadInstalled()
    logger.info {
        "sento-host: ${ExtensionManager.installedExtensions.size} extensions, data at ${HostConfig.dataRoot}"
    }

    buildApi().start(HostConfig.port)
}
