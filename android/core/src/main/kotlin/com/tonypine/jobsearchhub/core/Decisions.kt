package com.tonypine.jobsearchhub.core

import kotlinx.serialization.Serializable

/** A knowledge-base entry a brief cites. */
@Serializable
data class CitedEntry(val id: String, val title: String, val organization: String? = null) {
    /** The entry as a citation reads: its title, and where when it has one. */
    val label: String get() = if (organization.isNullOrEmpty()) title else "$title · $organization"
}

/** One strength or weakness, with the entries it rests on. */
@Serializable
data class JobBriefPoint(val point: String, val entryIds: List<String> = emptyList())

/** A job's brief: the match, why, and the strengths and weaknesses behind it. */
@Serializable
data class JobBrief(
    val tier: String,
    val model: String,
    val match: String,
    val reason: String,
    val strengths: List<JobBriefPoint> = emptyList(),
    val weaknesses: List<JobBriefPoint> = emptyList(),
    val isStale: Boolean = false,
    val citedEntries: List<CitedEntry> = emptyList(),
) {
    val isFull: Boolean get() = tier == "full"

    /** The cited entries a point rests on; entries since deleted are left out. */
    fun entriesOf(point: JobBriefPoint): List<CitedEntry> = point.entryIds.mapNotNull { id -> citedEntries.firstOrNull { it.id == id } }
}

/** One reason a posting could screen the owner out at once; no verdict is information the fit doesn't judge. */
@Serializable
data class ScreenOutAnswer(val name: String, val verdict: String? = null, val answer: String, val evidence: String? = null)

/** The owner's latest decision on a job: pursue, skip or later. */
@Serializable
data class JobDecision(val decision: String, val reason: String? = null, val decidedAt: String)

@Serializable
data class JobDecisionRequest(val decision: String, val reason: String)

/** A briefed job waiting for the owner's decision. */
@Serializable
data class DecisionQueueItem(
    val job: Job,
    val companyName: String? = null,
    val match: String,
    val reason: String,
    val briefTier: String,
    val fit: JobFit,
    val decision: JobDecision? = null,
)

@Serializable
data class DecisionQueueResponse(val items: List<DecisionQueueItem>, val total: Int)
