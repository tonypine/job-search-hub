package com.tonypine.jobsearchhub.core

/** The jobs the phone lists: good fits, then unclear ones when asked, newest first. */
object JobsOrder {
    private val rank = mapOf("good" to 0, "unclear" to 1, "poor" to 2)

    fun pick(items: List<JobListItem>, includeUnclear: Boolean): List<JobListItem> =
        items
            .filter { it.fit.level == "good" || (includeUnclear && it.fit.level == "unclear") }
            .sortedWith(compareBy<JobListItem> { rank[it.fit.level] ?: 3 }.thenByDescending { it.job.firstSeenAt })

    /** The line under Jobs' title: "84 open", and how many the list shows when it shows fewer. */
    fun summary(openCount: Int, shown: Int): String = if (shown < openCount) "$openCount open · $shown shown" else "$openCount open"
}
