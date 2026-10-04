package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class PushedUpdateTest {
    @Test
    fun aPushReadsAsTheUpdateItCarries() {
        val update = PushedUpdate.parse(mapOf("update_id" to "u1", "title" to "Acme replied", "body" to "They'd like a call.", "company_id" to "c1"))
        assertEquals(PushedUpdate("u1", "Acme replied", "They'd like a call.", jobId = null, companyId = "c1"), update)
    }

    @Test
    fun aFollowUpReminderSaysItsKind() {
        val update = PushedUpdate.parse(mapOf("update_id" to "u1", "kind" to "follow_up_due", "title" to "Follow up with Acme", "job_id" to "j1"))
        assertEquals("follow_up_due", update?.kind)
        assertTrue(update?.isFollowUp == true)
        assertFalse(PushedUpdate.parse(mapOf("update_id" to "u1", "title" to "Acme replied"))!!.isFollowUp)
    }

    @Test
    fun aPushWithoutAnUpdateOrTitleIsNotOne() {
        assertNull(PushedUpdate.parse(mapOf("title" to "Acme replied")))
        assertNull(PushedUpdate.parse(mapOf("update_id" to "u1", "title" to " ")))
    }
}
