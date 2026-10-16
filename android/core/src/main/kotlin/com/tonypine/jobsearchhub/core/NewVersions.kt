package com.tonypine.jobsearchhub.core

import kotlinx.serialization.Serializable
import java.time.Duration
import java.time.Instant

/** A release on GitHub, as `GET /repos/<repo>/releases` lists it. */
@Serializable
data class GitHubRelease(
    val tagName: String,
    val body: String? = null,
    val draft: Boolean = false,
    /** A pre-release is a withdrawn version, which the phone doesn't offer. */
    val prerelease: Boolean = false,
    val assets: List<ReleaseAsset> = emptyList(),
) {
    fun asset(name: String): ReleaseAsset? = assets.firstOrNull { it.name == name }
}

@Serializable
data class ReleaseAsset(val name: String, val browserDownloadUrl: String, val size: Long = 0)

/** A version of the app, `0.1.<commit count>`, as the release tags and `versionName` carry it. */
data class AppVersion(val major: Int, val minor: Int, val patch: Int) : Comparable<AppVersion> {
    override fun compareTo(other: AppVersion): Int = compareValuesBy(this, other, { it.major }, { it.minor }, { it.patch })

    override fun toString(): String = "$major.$minor.$patch"

    companion object {
        private val pattern = Regex("""(\d+)\.(\d+)\.(\d+)(-.*)?""")

        /** `0.1.253`, or a local build's `0.1.0-dev.abc1234`, which reads as 0.1.0, older than every release. */
        fun parse(text: String): AppVersion? {
            val (major, minor, patch) = pattern.matchEntire(text.trim())?.destructured ?: return null
            return AppVersion(major.toInt(), minor.toInt(), patch.toInt())
        }
    }
}

/**
 * A release's notes, as `scripts/release/changelog.sh` writes them: lines under New, Fixed and Other changes, each
 * without the parts of the hub it names ("Phone: ") or its pull request ("(#64)"), which the phone doesn't show.
 */
@Serializable
data class Changelog(val new: List<String> = emptyList(), val fixed: List<String> = emptyList(), val other: List<String> = emptyList()) {
    /** What Today's card shows: the first two New lines, or Fixed ones when there are none. */
    fun highlights(count: Int = 2): List<String> = new.ifEmpty { fixed }.take(count)

    operator fun plus(later: Changelog): Changelog = Changelog(new + later.new, fixed + later.fixed, other + later.other)

    companion object {
        private val tags = Regex("""^(?:Mac|Server|Phone)(?:, (?:Mac|Server|Phone))*: """)
        private val pullRequest = Regex("""\s*\(#\d+\)$""")

        fun parse(markdown: String): Changelog {
            val sections = mapOf("new" to mutableListOf<String>(), "fixed" to mutableListOf(), "other changes" to mutableListOf())
            var section: MutableList<String>? = null
            for (raw in markdown.lineSequence()) {
                val line = raw.trim()
                when {
                    line.startsWith("#") -> section = sections[line.trimStart('#').trim().lowercase()]
                    line.startsWith("- ") || line.startsWith("* ") ->
                        section?.add(line.substring(2).trim().replace(tags, "").replace(pullRequest, "").trim())
                }
            }
            return Changelog(sections.getValue("new"), sections.getValue("fixed"), sections.getValue("other changes"))
        }
    }
}

/** The newest release the phone can install, with what changed since its own version. */
data class NewVersion(val version: AppVersion, val apk: ReleaseAsset, val checksum: ReleaseAsset, val changelog: Changelog)

/** A new version downloaded and checked, which the phone offers, and since when. */
@Serializable
data class ReadyVersion(val version: String, val changelog: Changelog, val readySince: String, val apkPath: String)

/** When the owner tapped Later on a version's card. */
@Serializable
data class Postponed(val version: String, val at: String)

/** How the phone finds its new versions among the repository's releases, and when it shows and tells of one. */
object NewVersions {
    const val TAG_PREFIX = "android-v"

    /** How long Later hides a version's card, unless a newer one comes first. */
    val LATER: Duration = Duration.ofDays(3)

    /** How long a ready version waits before a notification on Hub says so, once. */
    val WAITED: Duration = Duration.ofDays(7)

    fun apkName(version: AppVersion): String = "job-search-hub-$version.apk"

    fun checksumName(version: AppVersion): String = apkName(version) + ".sha256"

    /**
     * The newest Android release above [installed] that has its APK and checksum, skipping drafts, pre-releases
     * (withdrawn) and the versions in [bad] that failed their checks here, with the changelogs of every release from
     * there down to [installed], newest first.
     */
    fun pick(releases: List<GitHubRelease>, installed: AppVersion, bad: Set<String> = emptySet()): NewVersion? {
        val newer = releases
            .filter { !it.draft && !it.prerelease && it.tagName.startsWith(TAG_PREFIX) }
            .mapNotNull { release -> AppVersion.parse(release.tagName.removePrefix(TAG_PREFIX))?.let { it to release } }
            .filter { (version, _) -> version > installed }
            .sortedByDescending { it.first }
        for ((version, release) in newer) {
            if (version.toString() in bad) continue
            val apk = release.asset(apkName(version)) ?: continue
            val checksum = release.asset(checksumName(version)) ?: continue
            val changelog = newer.filter { it.first <= version }.map { Changelog.parse(it.second.body.orEmpty()) }.fold(Changelog(), Changelog::plus)
            return NewVersion(version, apk, checksum, changelog)
        }
        return null
    }

    /** The SHA-256 a `.sha256` asset names, as `sha256sum` writes it: the hex digest, then the file's name. */
    fun parseChecksum(text: String): String? =
        text.trim().split(Regex("""\s+""")).firstOrNull()?.lowercase()?.takeIf { it.matches(Regex("[0-9a-f]{64}")) }

    /** Today shows a ready version's card unless Later hid that version, or an older one, less than three days ago. */
    fun showsCard(ready: AppVersion, postponed: Postponed?, now: Instant): Boolean {
        postponed ?: return true
        val hidden = AppVersion.parse(postponed.version) ?: return true
        return ready > hidden || !now.isBefore(Instant.parse(postponed.at).plus(LATER))
    }

    /** A ready version that has waited a week gets one notification, never a second for the same version. */
    fun remindsOfWaiting(ready: ReadyVersion, remindedVersion: String?, now: Instant): Boolean =
        ready.version != remindedVersion && !now.isBefore(Instant.parse(ready.readySince).plus(WAITED))

    /** The notification on Hub for a version that has waited a week. */
    fun waitingNotice(version: String): Notice = Notice("Version $version is ready", "It has waited a week. Tap to update the app.")

    /** The notification on Hub when the hub no longer serves the app, with the hub's own words. */
    fun tooOldNotice(message: String): Notice = Notice(TOO_OLD, message)

    const val TOO_OLD = "This app is too old for your hub"
}
