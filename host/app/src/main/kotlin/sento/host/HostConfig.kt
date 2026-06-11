package sento.host

import java.io.File

// Host configuration, environment-driven like the Go side (no config
// file: Sento owns domain settings, the host gets infrastructure knobs).
object HostConfig {
    val port: Int = env("SENTO_HOST_PORT", "8901").toInt()
    val dataRoot: String = File(env("SENTO_HOST_DATA", "data")).absolutePath

    // Extension repo index URLs (keiyoushi by default). Comma-separated.
    val repos: List<String> =
        env(
            "SENTO_HOST_REPOS",
            "https://raw.githubusercontent.com/keiyoushi/extensions/repo/index.min.json",
        ).split(',').map { it.trim() }.filter { it.isNotEmpty() }

    // Optional FlareSolverr for Cloudflare-protected sources.
    val flareSolverrUrl: String = env("SENTO_HOST_FLARESOLVERR_URL", "")

    private fun env(
        key: String,
        default: String,
    ): String = System.getenv(key)?.takeIf { it.isNotBlank() } ?: default
}
