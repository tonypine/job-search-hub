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

    @Test
    fun anUpdateCarriedAsDataReadsBackTheSame() {
        val update = PushedUpdate("u1", "Follow up with Acme", "In Applied since Sep 25.", jobId = "j1", companyId = "c1", kind = "follow_up_due")
        assertEquals(update, PushedUpdate.parse(update.toData()))
        val bare = PushedUpdate("u2", "The Mac finished your request", body = null, jobId = null, companyId = null)
        assertEquals(mapOf("update_id" to "u2", "title" to "The Mac finished your request"), bare.toData())
        assertEquals(bare, PushedUpdate.parse(bare.toData()))
    }
}
