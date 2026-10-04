package com.tonypine.jobsearchhub.data

import com.tonypine.jobsearchhub.core.Pairing
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class HubClientTest {
    @Test
    fun theClientSendsTheDeviceTokenAndReadsTheJobs() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body("""{"jobs":[{"job":{"id":"1","source":"x","title":"Engineer","url":"https://x","first_seen_at":"2026-09-29T10:00:00Z"},"fit":{"level":"good"}}],"total":1}""").build())
            server.start()
            val client = HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test"))

            val jobs = client.getJobs()

            assertEquals("Engineer", jobs.jobs.single().job.title)
            val request = server.takeRequest()
            assertEquals("Bearer hubdev_test", request.headers["Authorization"])
            assertTrue(request.target.startsWith("/v1/jobs?status=open"))
            assertEquals(1, server.requestCount)
        }
    }

    @Test
    fun everyOpenJobIsReadAPageAtATime() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body(jobsPage(listOf("a", "b"), total = 3)).build())
            server.enqueue(MockResponse.Builder().body(jobsPage(listOf("c"), total = 3)).build())
            server.start()
            val client = HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test"))

            val jobs = client.getJobs()

            assertEquals(listOf("a", "b", "c"), jobs.jobs.map { it.job.title })
            assertEquals(3, jobs.total)
            assertEquals("/v1/jobs?status=open&limit=500", server.takeRequest().target)
            assertEquals("/v1/jobs?status=open&limit=500&offset=2", server.takeRequest().target)
            assertEquals(2, server.requestCount)
        }
    }

    @Test
    fun anEmptyPageEndsTheReadingEvenShortOfTheTotal() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body(jobsPage(listOf("a"), total = 2)).build())
            server.enqueue(MockResponse.Builder().body(jobsPage(emptyList(), total = 2)).build())
            server.start()
            val client = HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test"))

            val jobs = client.getJobs()

            assertEquals(listOf("a"), jobs.jobs.map { it.job.title })
            assertEquals(2, server.requestCount)
        }
    }

    @Test
    fun thePhoneRegistersItsPushToken() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().code(204).build())
            server.start()
            HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test")).setPushToken("fcm-token")

            val request = server.takeRequest()
            assertEquals("PUT", request.method)
            assertEquals("/v1/devices/me/push-token", request.target)
            assertEquals("""{"token":"fcm-token"}""", request.body?.utf8())
        }
    }

    @Test
    fun aDecisionIsSentWithItsReason() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body("""{"decision":"skip","reason":"agency","decided_at":"2026-09-30T21:00:00Z"}""").build())
            server.start()
            val decision = HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test")).decideJob("7", "skip", "  agency ")

            val request = server.takeRequest()
            assertEquals("POST", request.method)
            assertEquals("/v1/jobs/7/decision", request.target)
            assertEquals("""{"decision":"skip","reason":"agency"}""", request.body?.utf8())
            assertEquals("skip", decision.decision)
        }
    }

    @Test
    fun theDecisionQueueIsRead() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body("""{"items":[{"job":{"id":"1","source":"x","title":"Engineer","url":"https://x","first_seen_at":"2026-09-29T10:00:00Z"},
                "company_name":"Acme","match":"strong","reason":"React.","brief_tier":"pre","fit":{"level":"good"}}],"total":1}""").build())
            server.start()
            val queue = HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_test")).getDecisionQueue()

            assertEquals("strong", queue.items.single().match)
            assertEquals("/v1/decision-queue", server.takeRequest().target)
        }
    }

    @Test
    fun aRefusedTokenSaysToPairAgain() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().code(401).build())
            server.start()
            val error = assertFailsWith<HubException> { HubClient(Pairing(server.url("/").toString().trimEnd('/'), "hubdev_old")).getUpdates() }
            assertTrue(error.isRefused)
        }
    }

    private fun jobsPage(titles: List<String>, total: Int): String {
        val jobs = titles.joinToString(",") { title ->
            """{"job":{"id":"$title","source":"x","title":"$title","url":"https://x/$title","first_seen_at":"2026-09-29T10:00:00Z"},"fit":{"level":"good"}}"""
        }
        return """{"jobs":[$jobs],"total":$total}"""
    }
}
