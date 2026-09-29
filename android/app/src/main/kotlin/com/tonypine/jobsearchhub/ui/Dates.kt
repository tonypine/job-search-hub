package com.tonypine.jobsearchhub.ui

import android.text.format.DateUtils
import java.time.Instant

/** "5 minutes ago", from the hub's ISO timestamp. */
fun formatWhen(timestamp: String): String =
    runCatching {
        DateUtils.getRelativeTimeSpanString(Instant.parse(timestamp).toEpochMilli(), System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS).toString()
    }.getOrDefault(timestamp)
