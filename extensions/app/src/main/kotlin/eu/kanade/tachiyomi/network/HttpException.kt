/*
 * Adapted from Suwayomi-Server (Copyright Contributors to the Suwayomi
 * project, MPL-2.0), preserving Retry-After across the extension boundary.
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package eu.kanade.tachiyomi.network

import okhttp3.Response

/**
 * Exception that handles HTTP codes considered not successful by OkHttp.
 * Use it to have a standardized error message in the app across the extensions.
 *
 * @see Response.isSuccessful
 * @since tachiyomix 1.6
 * @param code [Int] the HTTP status code
 */
class HttpException @JvmOverloads constructor(
    val code: Int,
    val retryAfter: String? = null,
) : IllegalStateException("HTTP error $code")
