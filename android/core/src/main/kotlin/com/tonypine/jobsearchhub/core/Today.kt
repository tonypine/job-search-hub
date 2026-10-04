package com.tonypine.jobsearchhub.core

import kotlinx.serialization.Serializable

/** A LinkedIn conversation a recruiter started, with what their company has open now. */
@Serializable
data class RecruiterConversation(
    val id: String,
    val startedByName: String = "",
    val ownerWrote: Boolean = false,
    val lastMessageAt: String? = null,
    val hiringCompany: String? = null,
    val role: String? = null,
    val isAgency: Boolean = false,
    val companyId: String? = null,
    val openJobs: Int = 0,
    val fittingJobs: Int = 0,
) {
    /** Unanswered, at a company with jobs that fit: worth a reply. */
    val isWaiting: Boolean get() = !ownerWrote && fittingJobs > 0
}

@Serializable
data class RecruitersResponse(val recruiters: List<RecruiterConversation> = emptyList())

@Serializable
data class MarkUpdatesSeenRequest(val ids: List<String>)

/** What Today, the page that answers "what should I do now?", takes from the hub's lists. */
object Today {
    /** Updates about someone writing back. */
    private val replyKinds = setOf("human_reply", "interview_invite", "rejection", "recruiter_outreach")

    /** Updates that aren't news on Today: follow-up reminders show as Follow up, and the phone's own requests it already knows. */
    private val hiddenKinds = setOf(HubUpdate.FOLLOW_UP_DUE, "task_queued")

    /** The unseen updates Today lists, newest first as the hub sends them. */
    fun unseenUpdates(updates: List<HubUpdate>): List<HubUpdate> = updates.filter { it.seenAt == null && it.kind !in hiddenKinds }

    /** The line under Today's title: "7 to decide · 1 overdue · 1 due today · 2 replies". */
    fun summary(toDecide: Int, followUps: List<FollowUpStatus>, updates: List<HubUpdate>): String {
        val overdue = followUps.count { it.due == FollowUpDue.OVERDUE }
        val dueToday = followUps.count { it.due == FollowUpDue.TODAY }
        val replies = unseenUpdates(updates).count { it.kind in replyKinds }
        val parts = listOfNotNull(
            "$toDecide to decide".takeIf { toDecide > 0 },
            "$overdue overdue".takeIf { overdue > 0 },
            "$dueToday due today".takeIf { dueToday > 0 },
            (if (replies == 1) "1 reply" else "$replies replies").takeIf { replies > 0 },
        )
        return parts.joinToString(" · ").ifEmpty { "Nothing needs you now" }
    }
}
