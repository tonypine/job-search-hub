package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class PushedUpdateTest {
    @Test
    fun aPushReadsAsTheUpdateItCarries() {
        val update = PushedUpdate.parse(mapOf("update_id" to "u1", "title" to "Acme replied", "body" to "They'd like a call.", "company_id" to "c1"))
        assertEquals(PushedUpdate("u1", "Acme replied", "They'd like a call.", jobId = null, companyId = "c1"), update)
    }

    @Test
    fun aPushWithoutAnUpdateOrTitleIsNotOne() {
        assertNull(PushedUpdate.parse(mapOf("title" to "Acme replied")))
        assertNull(PushedUpdate.parse(mapOf("update_id" to "u1", "title" to " ")))
    }
}
