package com.tonypine.jobsearchhub.core

import java.net.URI
import java.net.URLDecoder

/** Where the hub is and the phone's token, from the Mac's pairing QR code. */
data class Pairing(val hubUrl: String, val token: String)

/**
 * The link a pairing QR code carries, `jobsearchhub://pair?url=…&token=…`,
 * as the Mac app's Settings makes it.
 */
object PairingLink {
    fun parse(link: String): Pairing? {
        val uri = runCatching { URI(link.trim()) }.getOrNull() ?: return null
        if (uri.scheme != "jobsearchhub" || uri.host != "pair") return null
        val query = uri.rawQuery ?: return null
        val values = query.split("&").mapNotNull { pair ->
            val parts = pair.split("=", limit = 2)
            if (parts.size != 2) null else URLDecoder.decode(parts[0], Charsets.UTF_8) to URLDecoder.decode(parts[1], Charsets.UTF_8)
        }.toMap()
        val hubUrl = values["url"]?.trimEnd('/').orEmpty()
        val token = values["token"].orEmpty()
        if (hubUrl.isEmpty() || token.isEmpty()) return null
        return Pairing(hubUrl, token)
    }
}
