package com.tonypine.jobsearchhub.core

/** The jobs the phone lists: good fits, then unclear ones when asked, newest first. */
object JobsOrder {
    private val rank = mapOf("good" to 0, "unclear" to 1, "poor" to 2)

    fun pick(items: List<JobListItem>, includeUnclear: Boolean): List<JobListItem> =
        items
            .filter { it.fit.level == "good" || (includeUnclear && it.fit.level == "unclear") }
            .sortedWith(compareBy<JobListItem> { rank[it.fit.level] ?: 3 }.thenByDescending { it.job.firstSeenAt })
}

// CI check (TP-399): an Android-only change; this PR is closed unmerged.
