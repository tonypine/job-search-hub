package com.tonypine.jobsearchhub.core

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class NewVersionsTest {
    private fun release(tag: String, body: String = "", draft: Boolean = false, prerelease: Boolean = false, withChecksum: Boolean = true): GitHubRelease {
        val version = tag.substringAfter("-v")
        val assets = buildList {
            if (tag.startsWith("android-v")) {
                add(ReleaseAsset("job-search-hub-$version.apk", "https://example.com/$tag/job-search-hub-$version.apk", 12_000_000))
                if (withChecksum) add(ReleaseAsset("job-search-hub-$version.apk.sha256", "https://example.com/$tag/job-search-hub-$version.apk.sha256"))
            } else {
                add(ReleaseAsset("Job-Search-Hub-$version.zip", "https://example.com/$tag/Job-Search-Hub-$version.zip"))
            }
        }
        return GitHubRelease(tag, body, draft, prerelease, assets)
    }

    private val installed = AppVersion(0, 1, 248)

    @Test
    fun versionsReadFromTagsAndLocalBuilds() {
        assertEquals(AppVersion(0, 1, 253), AppVersion.parse("0.1.253"))
        assertEquals(AppVersion(0, 1, 0), AppVersion.parse("0.1.0-dev.abc1234"))
        assertNull(AppVersion.parse("0.1"))
        assertNull(AppVersion.parse("latest"))
        assertTrue(AppVersion(0, 1, 253) > AppVersion(0, 1, 99))
        assertTrue(AppVersion(0, 2, 1) > AppVersion(0, 1, 300))
        assertEquals("0.1.253", AppVersion(0, 1, 253).toString())
    }

    @Test
    fun picksTheNewestAndroidReleaseAboveTheInstalledOne() {
        val releases = listOf(
            release("mac-v0.1.260"),
            release("android-v0.1.250"),
            release("android-v0.1.253"),
            release("android-v0.1.248"),
            release("android-v0.1.240"),
        )
        val picked = NewVersions.pick(releases, installed)!!
        assertEquals(AppVersion(0, 1, 253), picked.version)
        assertEquals("job-search-hub-0.1.253.apk", picked.apk.name)
        assertEquals("job-search-hub-0.1.253.apk.sha256", picked.checksum.name)
    }

    @Test
    fun skipsDraftsWithdrawnReleasesBadVersionsAndReleasesWithoutAChecksum() {
        val releases = listOf(
            release("android-v0.1.256", draft = true),
            release("android-v0.1.255", prerelease = true),
            release("android-v0.1.254", withChecksum = false),
            release("android-v0.1.253"),
            release("android-v0.1.250"),
        )
        assertEquals(AppVersion(0, 1, 253), NewVersions.pick(releases, installed)?.version)
        assertEquals(AppVersion(0, 1, 250), NewVersions.pick(releases, installed, bad = setOf("0.1.253"))?.version)
        assertNull(NewVersions.pick(releases, installed, bad = setOf("0.1.253", "0.1.250")))
    }

    @Test
    fun offersNothingWhenTheAppIsUpToDate() {
        assertNull(NewVersions.pick(listOf(release("android-v0.1.248"), release("android-v0.1.200"), release("mac-v0.1.300")), installed))
        assertNull(NewVersions.pick(emptyList(), installed))
    }

    @Test
    fun aLocalBuildIsOfferedEveryRelease() {
        assertEquals(AppVersion(0, 1, 3), NewVersions.pick(listOf(release("android-v0.1.3")), AppVersion.parse("0.1.0-dev.abc1234")!!)?.version)
    }

    @Test
    fun whatsNewGathersEveryReleaseSinceTheInstalledOneNewestFirst() {
        val releases = listOf(
            release("android-v0.1.253", "## New\n\n- Phone: Snooze a follow-up from its notification (#80)\n\n## Fixed\n\n- Phone: Keep the Decide list's place after a swipe (#79)\n"),
            release("android-v0.1.250", "## New\n\n- Phone: Show a job's salary range on its card (#75)\n"),
            release("android-v0.1.248", "## New\n\n- Phone: Already installed (#70)\n"),
        )
        val changelog = NewVersions.pick(releases, installed)!!.changelog
        assertEquals(listOf("Snooze a follow-up from its notification", "Show a job's salary range on its card"), changelog.new)
        assertEquals(listOf("Keep the Decide list's place after a swipe"), changelog.fixed)
    }

    @Test
    fun parsesTheReleaseNotesFormat() {
        val changelog = Changelog.parse(
            """
            ## New

            - Phone: Snooze a follow-up from its notification (#80)
            - Mac, Server, Phone: Show a job's salary range on its card (#75)

            ## Fixed

            - Phone: Took out: swipe a card to follow up (#64) (#70)
            - Keep the Decide list's place after a swipe

            ## Other changes

            - Phone: chore: bump Compose (#72)
            """.trimIndent(),
        )
        assertEquals(listOf("Snooze a follow-up from its notification", "Show a job's salary range on its card"), changelog.new)
        assertEquals(listOf("Took out: swipe a card to follow up (#64)", "Keep the Decide list's place after a swipe"), changelog.fixed)
        assertEquals(listOf("chore: bump Compose"), changelog.other)
        assertEquals(changelog.new, changelog.highlights())
    }

    @Test
    fun theCardFallsBackToFixedLinesAndAnUnknownSectionIsLeftOut() {
        val changelog = Changelog.parse("## Fixed\n\n- Phone: One (#1)\n- Phone: Two (#2)\n- Phone: Three (#3)\n\n## Later\n\n- Phone: Not a section (#4)\n")
        assertEquals(listOf("One", "Two"), changelog.highlights())
        assertEquals(emptyList(), changelog.new)
        assertEquals(emptyList(), Changelog.parse("No changes under android/ since android-v0.1.20.").highlights())
    }

    @Test
    fun laterHidesTheCardForThreeDaysOrUntilTheNextRelease() {
        val at = Instant.parse("2026-10-06T09:00:00Z")
        val later = Postponed("0.1.253", at.toString())
        val ready = AppVersion(0, 1, 253)
        assertTrue(NewVersions.showsCard(ready, null, at))
        assertFalse(NewVersions.showsCard(ready, later, at.plusSeconds(60)))
        assertFalse(NewVersions.showsCard(ready, later, at.plus(NewVersions.LATER).minusSeconds(1)))
        assertTrue(NewVersions.showsCard(ready, later, at.plus(NewVersions.LATER)))
        assertTrue(NewVersions.showsCard(AppVersion(0, 1, 254), later, at.plusSeconds(60)))
        assertFalse(NewVersions.showsCard(AppVersion(0, 1, 252), later, at.plusSeconds(60)))
    }

    @Test
    fun aReadyVersionThatWaitedAWeekIsToldOfOnce() {
        val since = Instant.parse("2026-10-01T09:00:00Z")
        val ready = ReadyVersion("0.1.253", Changelog(), since.toString(), "/data/versions/job-search-hub-0.1.253.apk")
        assertFalse(NewVersions.remindsOfWaiting(ready, null, since.plus(NewVersions.WAITED).minusSeconds(1)))
        assertTrue(NewVersions.remindsOfWaiting(ready, null, since.plus(NewVersions.WAITED)))
        assertTrue(NewVersions.remindsOfWaiting(ready, "0.1.250", since.plus(NewVersions.WAITED)))
        assertFalse(NewVersions.remindsOfWaiting(ready, "0.1.253", since.plus(NewVersions.WAITED)))
        assertEquals("Version 0.1.253 is ready", NewVersions.waitingNotice("0.1.253").title)
        assertEquals(Notice("This app is too old for your hub", "Update the app."), NewVersions.tooOldNotice("Update the app."))
    }

    @Test
    fun readsTheChecksumFileSha256sumWrites() {
        val digest = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
        assertEquals(digest, NewVersions.parseChecksum("$digest  job-search-hub-0.1.253.apk\n"))
        assertEquals(digest, NewVersions.parseChecksum(digest.uppercase()))
        assertNull(NewVersions.parseChecksum("not a digest  job-search-hub-0.1.253.apk"))
        assertNull(NewVersions.parseChecksum(""))
    }

    @Test
    fun readsGitHubsReleasesJson() {
        val releases = hubJson.decodeFromString<List<GitHubRelease>>(
            """[{"tag_name":"android-v0.1.253","name":"Android 0.1.253","draft":false,"prerelease":false,"body":"## New\n\n- Phone: X (#1)",
               "assets":[{"name":"job-search-hub-0.1.253.apk","browser_download_url":"https://github.com/a.apk","size":42,"content_type":"x"}]}]""",
        )
        assertEquals("android-v0.1.253", releases.single().tagName)
        assertEquals("https://github.com/a.apk", releases.single().assets.single().browserDownloadUrl)
        assertEquals(42, releases.single().assets.single().size)
    }
}
