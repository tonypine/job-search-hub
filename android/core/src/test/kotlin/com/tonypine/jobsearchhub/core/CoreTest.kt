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
    fun aFixRequestCarriesTheJobAndTheNote() {
        val encoded = hubJson.encodeToString(QueueTaskRequest.serializer(), QueueTaskRequest(kind = "fix_job", jobId = "j1", note = "the city is Lisbon"))
        assertEquals("""{"kind":"fix_job","job_id":"j1","note":"the city is Lisbon"}""", encoded)
        val task = hubJson.decodeFromString<TaskRequest>("""{"id":"t1","kind":"fix_job","status":"queued","job_id":"j1","input":"the city is Lisbon"}""")
        assertEquals("j1", task.jobId)
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

    @Test
    fun theHubAvatarNamesTheHostOrElseTheHub() {
        assertEquals("mac", Pairing("https://mac.tailnet.ts.net", "t").hubName)
        assertEquals("localhost", Pairing("http://localhost:8080", "t").hubName)
        assertEquals("Hub", Pairing("http://10.0.2.2:8080", "t").hubName)
        assertEquals("Hub", Pairing("http://[::1]:8080", "t").hubName)
        assertEquals("Hub", Pairing("not a url", "t").hubName)
    }

    @Test
    fun theSummariesUnderDecideAndJobsCountTheirLists() {
        assertEquals("7 to decide", Decide.summary(7))
        assertEquals("Nothing to decide", Decide.summary(0))
        assertEquals("84 open", JobsOrder.summary(84, 84))
        assertEquals("84 open · 12 shown", JobsOrder.summary(84, 12))
        assertEquals("0 open", JobsOrder.summary(0, 0))
    }
}

class JobBriefTest {
    @Test
    fun aJobsDetailsCarryItsBriefScreenOutAnswersAndDecision() {
        val details = hubJson.decodeFromString<JobDetails>(
            """{"job":{"id":"1","source":"x","title":"Engineer","url":"https://x","first_seen_at":"2026-09-29T10:00:00Z"},"fit":{"level":"good"},
            "brief":{"tier":"full","model":"claude","match":"strong","reason":"React.","is_stale":true,
                     "strengths":[{"point":"React","entry_ids":["e1","gone"]}],"cited_entries":[{"id":"e1","title":"Built the platform","organization":"Maple"}]},
            "screen_out":[{"name":"Hires from Brazil","verdict":"yes","answer":"LATAM","evidence":"Remote in LATAM"},{"name":"Contract","answer":"not stated"}],
            "decision":{"decision":"later","decided_at":"2026-09-30T21:00:00Z"}}""",
        )
        val brief = details.brief!!
        kotlin.test.assertTrue(brief.isFull && brief.isStale)
        kotlin.test.assertEquals(listOf("Built the platform · Maple"), brief.entriesOf(brief.strengths.single()).map { it.label })
        kotlin.test.assertEquals(listOf("yes", null), details.screenOut.map { it.verdict })
        kotlin.test.assertEquals("later", details.decision?.decision)
    }
}

class CompanyBriefTest {
    @Test
    fun aCompanysCardsComeWithTheirPhase() {
        val board = hubJson.decodeFromString<PipelineBoard>(
            """{"phases":[{"id":"p1","name":"Saved","position":1,"is_closed":false},{"id":"p2","name":"Interviewing","position":4}],
            "cards":[{"application":{"id":"a1","company_id":"c1","phase_id":"p2","notes":"Panel on Friday","phase_entered_at":"2026-09-29T10:00:00Z"},"job_title":"Engineer","company_name":"Acme"},
            {"application":{"id":"a2","company_id":"c2","phase_id":"p1","phase_entered_at":"2026-09-29T10:00:00Z"},"company_name":"Other"}]}""",
        )
        val cards = board.findCards("c1")
        assertEquals(1, cards.size)
        assertEquals("Interviewing", cards.single().second)
        assertEquals("Panel on Friday", cards.single().first.application.notes)
    }
}
