// Stand-ins for the Suwayomi server symbols the vendored runtime layers
// reference (serverConfig, ApplicationDirs). Keeping the upstream package
// name means vendored files compile unchanged, so re-syncing them against
// a newer Suwayomi-Server stays a plain copy.
package suwayomi.tachidesk.server

import kotlinx.coroutines.flow.MutableStateFlow
import sento.host.HostConfig

class ServerConfig(
    val flareSolverrEnabled: MutableStateFlow<Boolean>,
    val flareSolverrUrl: MutableStateFlow<String>,
    val flareSolverrTimeout: MutableStateFlow<Int>,
    val flareSolverrSessionName: MutableStateFlow<String>,
    val flareSolverrSessionTtl: MutableStateFlow<Int>,
    val flareSolverrAsResponseFallback: MutableStateFlow<Boolean>,
)

val serverConfig: ServerConfig by lazy {
    ServerConfig(
        flareSolverrEnabled = MutableStateFlow(HostConfig.flareSolverrUrl.isNotBlank()),
        flareSolverrUrl = MutableStateFlow(HostConfig.flareSolverrUrl),
        flareSolverrTimeout = MutableStateFlow(60),
        flareSolverrSessionName = MutableStateFlow("sento-host"),
        flareSolverrSessionTtl = MutableStateFlow(15),
        flareSolverrAsResponseFallback = MutableStateFlow(false),
    )
}

class ApplicationDirs(
    val dataRoot: String = HostConfig.dataRoot,
    val extensionsRoot: String = "${HostConfig.dataRoot}/extensions",
)
