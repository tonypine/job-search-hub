package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class TodayTest {
    private fun update(id: String, kind: String, seenAt: String? = null) = HubUpdate(id = id, kind = kind, title = id, createdAt = "2026-10-04T12:00:00Z", seenAt = seenAt)

    private val updates = listOf(
        update("reply", "human_reply"),
        update("invite", "interview_invite"),
        update("seen reply", "human_reply", seenAt = "2026-10-04T13:00:00Z"),
        update("match", "fresh_match"),
        update("reminder", HubUpdate.FOLLOW_UP_DUE),
        update("asked", "task_queued"),
    )

    @Test
    fun todayListsTheUnseenUpdatesButNotRemindersOrThePhonesOwnRequests() {
        assertEquals(listOf("reply", "invite", "match"), Today.unseenUpdates(updates).map { it.id })
    }

    @Test
    fun theSummaryCountsWhatNeedsYou() {
        val followUps = listOf(FollowUpStatus(FollowUpDue.OVERDUE, 2), FollowUpStatus(FollowUpDue.TODAY, 0))
        assertEquals("7 to decide · 1 overdue · 1 due today · 2 replies", Today.summary(7, followUps, updates))
        assertEquals("1 reply", Today.summary(0, emptyList(), listOf(update("reply", "human_reply"))))
        assertEquals("Nothing needs you now", Today.summary(0, emptyList(), emptyList()))
    }

    @Test
    fun aRecruiterWaitsWhenUnansweredAtACompanyWithFittingJobs() {
        val recruiters = hubJson.decodeFromString<RecruitersResponse>(
            """{"recruiters":[{"id":"r1","started_by_name":"Sam","owner_wrote":false,"hiring_company":"Acme","company_id":"c1","open_jobs":3,"fitting_jobs":1,
            "message_count":2,"is_agency":false}]}""",
        ).recruiters
        assertTrue(recruiters.single().isWaiting)
        assertFalse(recruiters.single().copy(ownerWrote = true).isWaiting)
        assertFalse(recruiters.single().copy(fittingJobs = 0).isWaiting)
    }
}
