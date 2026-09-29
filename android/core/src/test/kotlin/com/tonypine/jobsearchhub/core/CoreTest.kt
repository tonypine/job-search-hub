package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class CoreTest {
    @Test
    fun aPairingLinkGivesTheHubAndTheToken() {
        val pairing = PairingLink.parse("jobsearchhub://pair?url=https://mac.tailnet.ts.net/&token=hubdev_abc%2B%2F%3D")
        assertEquals(Pairing("https://mac.tailnet.ts.net", "hubdev_abc+/="), pairing)
        assertNull(PairingLink.parse("https://example.com/?token=x"))
        assertNull(PairingLink.parse("jobsearchhub://pair?url=https://mac"))
    }

    @Test
    fun theHubsJsonReads() {
        val jobs = hubJson.decodeFromString<JobsResponse>(
            """{"jobs":[{"job":{"id":"1","source":"himalayas","title":"Senior Engineer","url":"https://x","first_seen_at":"2026-09-29T10:00:00Z",
            "surprise":true},"company_name":"Acme","unseen_updates":2,"fit":{"level":"good","checks":[{"name":"Role","verdict":"yes","reason":"Senior"}]}}],
            "total":1,"fact_columns":[]}""",
        )
        assertEquals("Acme", jobs.jobs.single().companyName)
        assertEquals(2, jobs.jobs.single().unseenUpdates)
        assertEquals("yes", jobs.jobs.single().fit.checks.single().verdict)
    }

    @Test
    fun goodFitsComeFirstThenUnclearOnesWhenAsked() {
        fun item(id: String, level: String, seen: String) =
            JobListItem(Job(id = id, source = "x", title = id, url = "https://x/$id", firstSeenAt = seen), fit = JobFit(level))
        val items = listOf(item("old-good", "good", "2026-09-01"), item("unclear", "unclear", "2026-09-29"),
            item("new-good", "good", "2026-09-28"), item("poor", "poor", "2026-09-29"))
        assertEquals(listOf("new-good", "old-good"), JobsOrder.pick(items, includeUnclear = false).map { it.job.id })
        assertEquals(listOf("new-good", "old-good", "unclear"), JobsOrder.pick(items, includeUnclear = true).map { it.job.id })
    }
}
