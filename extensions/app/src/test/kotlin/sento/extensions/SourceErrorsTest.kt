package sento.extensions

import eu.kanade.tachiyomi.network.HttpException
import eu.kanade.tachiyomi.network.asObservableSuccess
import eu.kanade.tachiyomi.source.model.Page
import eu.kanade.tachiyomi.source.online.HttpSource
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.Headers
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class SourceErrorsTest {
    private fun client() = OkHttpClient.Builder().addInterceptor { chain ->
        assertEquals("present", chain.request().header("X-Source-Header"))
        Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1)
            .code(429).message("Too Many Requests").header("Retry-After", "120")
            .body("slow down".toResponseBody()).build()
    }.build()

    @Test
    fun `source image requests retain HTTP status and retry delay`() = runBlocking {
        val source = object : HttpSource() {
            override val name = "Fake"
            override val lang = "en"
            override val supportsLatest = false
            override val baseUrl = "https://source.example"
            override fun headersBuilder() = Headers.Builder().add("X-Source-Header", "present")
            override val client = client()
        }
        val failure = assertFailsWith<HttpException> {
            source.getImage(Page(0, imageUrl = "https://source.example/page.jpg"))
        }
        val error = sourceError(failure)
        assertEquals(429, error.upstreamStatus)
        assertEquals("120", error.retryAfter)
        assertTrue(error.retryable)
        assertEquals("HTTP error 429", error.error)
    }

    @Suppress("DEPRECATION")
    @Test
    fun `legacy observable helper retains retry metadata too`() {
        val request = Request.Builder().url("https://source.example/image")
            .header("X-Source-Header", "present").build()
        val failure = assertFailsWith<HttpException> {
            client().newCall(request).asObservableSuccess().toBlocking().single()
        }
        assertEquals("120", failure.retryAfter)
        // Existing compiled extensions call the one-argument JVM constructor.
        val legacy = HttpException::class.java.getConstructor(Int::class.javaPrimitiveType).newInstance(401)
        assertEquals(401, legacy.code)
    }

    @Test
    fun `only network and temporary HTTP errors are retryable`() {
        for (status in listOf(408, 425, 429, 500, 502, 503, 504)) {
            assertTrue(sourceError(IllegalStateException("wrapped", HttpException(status))).retryable)
        }
        for (status in listOf(400, 401, 403, 404, 410)) {
            val error = sourceError(HttpException(status))
            assertFalse(error.retryable)
            assertEquals(status, error.upstreamStatus)
            // Explicit false must survive JSON encoding at the gateway.
            assertTrue(Json.encodeToString(error).contains("\"retryable\":false"))
        }
        assertTrue(sourceError(IOException("connection reset")).retryable)
        assertFalse(sourceError(IllegalArgumentException("invalid source key")).retryable)
        assertFalse(sourceError(IllegalArgumentException("invalid source key")).unreadable)
    }

    @Test
    fun `an IO-wrapped range header failure does not establish a damaged archive`() {
        val error = sourceError(IOException(IllegalArgumentException("Not a local file header")))
        assertTrue(error.retryable)
        assertFalse(error.unreadable)
        assertTrue(Json.encodeToString(error).contains("\"retryable\":true"))
    }

    @Test
    fun `an explicit ZIP format error remains unreadable`() {
        val error = sourceError(java.util.zip.ZipException("invalid entry size"))
        assertFalse(error.retryable)
        assertTrue(error.unreadable)
    }
}
