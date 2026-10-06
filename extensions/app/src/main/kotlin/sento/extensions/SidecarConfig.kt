package sento.extensions

import java.io.File

// Host configuration, environment-driven like the Go side (no config
// file: Sento owns domain settings, the host gets infrastructure knobs).
object SidecarConfig {
    val port: Int = env("SENTO_EXTENSIONS_PORT", "8901").toInt()
    val dataRoot: String = File(env("SENTO_EXTENSIONS_DATA", "data")).absolutePath

    // Extension repo index URLs, comma-separated. None ship by default —
    // the user adds Mihon-compatible repos (Sento pushes its configured
    // list on connect; this env var is an optional extra for standalone
    // use).
    val repos: List<String> =
        env("SENTO_EXTENSIONS_REPOS", "").split(',').map { it.trim() }.filter { it.isNotEmpty() }

    // Optional FlareSolverr for Cloudflare-protected sources.
    val flareSolverrUrl: String = env("SENTO_EXTENSIONS_FLARESOLVERR_URL", "")

    private fun env(
        key: String,
        default: String,
    ): String = System.getenv(key)?.takeIf { it.isNotBlank() } ?: default
}
