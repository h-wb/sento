package sento.extensions

import eu.kanade.tachiyomi.network.HttpException
import kotlinx.serialization.Serializable
import java.io.IOException

@Serializable
internal data class ErrorDto(
    val error: String,
    val retryable: Boolean,
    val upstreamStatus: Int? = null,
    val retryAfter: String? = null,
)

internal fun sourceError(error: Exception): ErrorDto {
    val causes = generateSequence<Throwable>(error) { it.cause }.take(16).toList()
    val http = causes.filterIsInstance<HttpException>().firstOrNull()
    return ErrorDto(
        error = error.message ?: error.javaClass.simpleName,
        retryable = if (http != null) {
            http.code in listOf(408, 425, 429) || http.code in 500..599
        } else {
            causes.any { it is IOException }
        },
        upstreamStatus = http?.code,
        retryAfter = http?.retryAfter,
    )
}
