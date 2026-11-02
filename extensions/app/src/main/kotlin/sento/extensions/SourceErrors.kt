package sento.extensions

import eu.kanade.tachiyomi.network.HttpException
import kotlinx.serialization.Serializable
import java.io.IOException
import java.util.zip.ZipException

@Serializable
internal data class ErrorDto(
    val error: String,
    val retryable: Boolean,
    val upstreamStatus: Int? = null,
    val retryAfter: String? = null,
    // The source served something that cannot be read (a damaged archive):
    // asking again gets the same bytes. Sento gives up on the release.
    val unreadable: Boolean = false,
)

internal fun sourceError(error: Exception): ErrorDto {
    val causes = generateSequence<Throwable>(error) { it.cause }.take(16).toList()
    val http = causes.filterIsInstance<HttpException>().firstOrNull()
    // An IO-wrapped header error can come from a transient range response,
    // even when the archive and its entry offsets are valid. It does not
    // establish a permanently unreadable release.
    val unreadable = http == null && causes.any { it is ZipException }
    return ErrorDto(
        error = error.message ?: error.javaClass.simpleName,
        retryable = if (http != null) {
            http.code in listOf(408, 425, 429) || http.code in 500..599
        } else {
            !unreadable && causes.any { it is IOException }
        },
        upstreamStatus = http?.code,
        retryAfter = http?.retryAfter,
        unreadable = unreadable,
    )
}
