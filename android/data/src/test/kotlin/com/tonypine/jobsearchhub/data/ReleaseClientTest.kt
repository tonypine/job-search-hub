package com.tonypine.jobsearchhub.data

import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import java.io.File
import java.io.IOException
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class ReleaseClientTest {
    @Test
    fun readsTheRepositorysReleases() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body("""[{"tag_name":"android-v0.1.253","draft":false,"prerelease":false,"body":"","assets":[]}]""").build())
            server.start()
            val client = ReleaseClient("owner/repo", server.url("/").toString().trimEnd('/'))

            assertEquals("android-v0.1.253", client.getReleases().single().tagName)
            val request = server.takeRequest()
            assertEquals("/repos/owner/repo/releases?per_page=100", request.target)
            assertEquals("application/vnd.github+json", request.headers["Accept"])
        }
    }

    @Test
    fun downloadsAnAssetWithItsSha256AndProgress() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().body("test").build())
            server.start()
            val file = File(Files.createTempDirectory("versions").toFile(), "job-search-hub-0.1.253.apk")
            val progress = mutableListOf<Float>()

            val digest = ReleaseClient("owner/repo", server.url("/").toString()).download(server.url("/a.apk").toString(), file) { progress += it }

            assertEquals("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", digest)
            assertEquals("test", file.readText())
            assertEquals(1f, progress.last())
        }
    }

    @Test
    fun aFailedDownloadLeavesNoFile() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().code(404).build())
            server.start()
            val file = File(Files.createTempDirectory("versions").toFile(), "job-search-hub-0.1.253.apk")
            file.writeText("partial")

            assertFailsWith<IOException> { ReleaseClient("owner/repo", server.url("/").toString()).download(server.url("/a.apk").toString(), file) }
            assertFalse(file.exists())
        }
    }

    @Test
    fun aRefusalSaysGitHubAnswered() = runTest {
        MockWebServer().use { server ->
            server.enqueue(MockResponse.Builder().code(403).build())
            server.start()

            val error = assertFailsWith<IOException> { ReleaseClient("owner/repo", server.url("/").toString().trimEnd('/')).getReleases() }
            assertTrue(error.message!!.contains("GitHub answered 403"), error.message)
        }
    }
}
