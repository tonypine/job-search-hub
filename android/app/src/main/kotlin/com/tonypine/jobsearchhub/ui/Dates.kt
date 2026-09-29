package com.tonypine.jobsearchhub.ui

import android.text.format.DateUtils
import java.time.Instant

/** "5 minutes ago", from the hub's ISO timestamp. */
fun formatWhen(timestamp: String): String =
    runCatching {
        val now = System.currentTimeMillis()
        // The hub's clock can run a little ahead of the phone's; a just-recorded update isn't in the future.
        val recordedAt = minOf(Instant.parse(timestamp).toEpochMilli(), now)
        DateUtils.getRelativeTimeSpanString(recordedAt, now, DateUtils.MINUTE_IN_MILLIS).toString()
    }.getOrDefault(timestamp)
