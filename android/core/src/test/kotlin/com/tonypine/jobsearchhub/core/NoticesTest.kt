package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class NoticesTest {
    private fun update(kind: String?, jobId: String? = "j1", companyId: String? = "c1") =
        PushedUpdate("u1", "Follow up with Acme", "In Applied since Sep 25.", jobId, companyId, kind)

    @Test
    fun eachKindOfUpdatePostsOnItsChannel() {
        assertEquals(NoticeChannel.FOLLOW_UPS, NoticeChannel.of("follow_up_due"))
        for (kind in listOf("reply", "human_reply", "interview_invite", "recruiter_outreach", "rejection", "application_confirmation")) {
            assertEquals(NoticeChannel.REPLIES, NoticeChannel.of(kind), kind)
        }
        assertEquals(NoticeChannel.MATCHES, NoticeChannel.of("fresh_match"))
        assertEquals(NoticeChannel.HUB, NoticeChannel.of("task_finished"))
        assertEquals(NoticeChannel.HUB, NoticeChannel.of("something_new"))
        assertEquals(NoticeChannel.HUB, NoticeChannel.of(null))
    }

    @Test
    fun theFourChannelsHaveTheirNamesAndIdsAndNoneReusesTheOldOne() {
        assertEquals(listOf("Follow-ups", "Replies", "Matches", "Hub"), NoticeChannel.entries.map { it.title })
        assertEquals(listOf("follow_ups", "replies", "matches", "hub"), NoticeChannel.entries.map { it.id })
        assertEquals(false, NoticeChannel.entries.any { it.id == NoticeChannel.LEGACY_ID })
        assertEquals(NoticeImportance.HIGH, NoticeChannel.FOLLOW_UPS.importance)
        assertEquals(NoticeImportance.DEFAULT, NoticeChannel.MATCHES.importance)
    }

    @Test
    fun aReminderRecordsOrSnoozesAndAReplyOpensOrIsMarkedRead() {
        assertEquals(listOf(NoticeAction.FOLLOWED_UP, NoticeAction.SNOOZE_A_DAY), update("follow_up_due").actions)
        assertEquals(listOf(NoticeAction.FOLLOWED_UP, NoticeAction.SNOOZE_A_DAY), update("follow_up_due", jobId = null).actions)
        assertEquals(listOf(NoticeAction.SNOOZE_A_DAY), update("follow_up_due", jobId = null, companyId = null).actions)
        assertEquals(listOf(NoticeAction.OPEN, NoticeAction.MARK_AS_READ), update("human_reply").actions)
        assertEquals(emptyList(), update("fresh_match").actions)
        assertEquals(emptyList(), update("task_finished").actions)
        assertEquals(listOf("Followed up", "Snooze a day", "Open", "Mark as read"), NoticeAction.entries.map { it.title })
    }

    @Test
    fun anActionReadsBackFromItsId() {
        for (action in NoticeAction.entries) {
            assertEquals(action, NoticeAction.of(action.id))
        }
        assertNull(NoticeAction.of("dismiss"))
        assertNull(NoticeAction.of(null))
    }

    @Test
    fun aGroupsSummaryCountsItsKind() {
        assertEquals(2, NoticeChannel.GROUP_FROM)
        assertEquals("2 fresh strong matches", NoticeChannel.MATCHES.summary(2))
        assertEquals("3 follow-ups due", NoticeChannel.FOLLOW_UPS.summary(3))
        assertEquals("2 replies", NoticeChannel.REPLIES.summary(2))
        assertEquals("4 hub updates", NoticeChannel.HUB.summary(4))
        assertEquals("1 reply", NoticeChannel.REPLIES.summary(1))
    }

    @Test
    fun aFailedActionSaysWhatFailedAndWhy() {
        assertEquals(
            Notice("Couldn't record the follow-up", "Follow up with Acme: Can't reach the hub at https://hub.example.ts.net: timeout"),
            NoticeAction.FOLLOWED_UP.failure(update("follow_up_due"), "Can't reach the hub at https://hub.example.ts.net: timeout"),
        )
        assertEquals(
            Notice("Couldn't record the follow-up", "Follow up with Acme: ${NoticeAction.NO_CARD}"),
            NoticeAction.FOLLOWED_UP.failure(update("follow_up_due"), NoticeAction.NO_CARD),
        )
        assertEquals("Couldn't mark the reply as read", NoticeAction.MARK_AS_READ.failure(update("human_reply"), "The hub answered 500.").title)
    }
}
