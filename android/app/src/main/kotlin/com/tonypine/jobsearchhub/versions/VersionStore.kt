package com.tonypine.jobsearchhub.versions

import android.content.Context
import androidx.core.content.edit
import com.tonypine.jobsearchhub.core.Postponed
import com.tonypine.jobsearchhub.core.ReadyVersion
import com.tonypine.jobsearchhub.core.hubJson
import java.time.Instant

/** What the phone remembers about its new versions between runs: the one ready, Later, the bad ones and what it told. */
class VersionStore(context: Context) {
    private val preferences = context.getSharedPreferences("versions", Context.MODE_PRIVATE)

    var ready: ReadyVersion?
        get() = preferences.getString(READY, null)?.let { runCatching { hubJson.decodeFromString<ReadyVersion>(it) }.getOrNull() }
        set(value) = preferences.edit { putString(READY, value?.let { hubJson.encodeToString(it) }) }

    var postponed: Postponed?
        get() = preferences.getString(POSTPONED, null)?.let { runCatching { hubJson.decodeFromString<Postponed>(it) }.getOrNull() }
        set(value) = preferences.edit { putString(POSTPONED, value?.let { hubJson.encodeToString(it) }) }

    /** The versions that failed their checks here, which aren't downloaded or offered again. */
    val bad: Set<String> get() = preferences.getStringSet(BAD, emptySet()).orEmpty()

    fun markBad(version: String) = preferences.edit { putStringSet(BAD, bad + version) }

    /** Why the last check, download or install didn't give a version, for Settings. */
    var problem: String?
        get() = preferences.getString(PROBLEM, null)
        set(value) = preferences.edit { putString(PROBLEM, value) }

    var checkedAt: Instant?
        get() = preferences.getString(CHECKED_AT, null)?.let(Instant::parse)
        set(value) = preferences.edit { putString(CHECKED_AT, value?.toString()) }

    /** The version an install was started for, so the next start on it says "Now on". */
    var installing: String?
        get() = preferences.getString(INSTALLING, null)
        set(value) = preferences.edit { putString(INSTALLING, value) }

    /** When the owner tapped Update and was sent to allow installs, which Android may restart the app for. */
    var askedForInstallsAt: Instant?
        get() = preferences.getString(ASKED_FOR_INSTALLS_AT, null)?.let(Instant::parse)
        set(value) = preferences.edit { putString(ASKED_FOR_INSTALLS_AT, value?.toString()) }

    /** The ready version a week-waited notification was posted for. */
    var remindedVersion: String?
        get() = preferences.getString(REMINDED_VERSION, null)
        set(value) = preferences.edit { putString(REMINDED_VERSION, value) }

    /** The installed version the too-old notification was posted for. */
    var toldTooOldFor: String?
        get() = preferences.getString(TOLD_TOO_OLD_FOR, null)
        set(value) = preferences.edit { putString(TOLD_TOO_OLD_FOR, value) }

    private companion object {
        const val READY = "ready"
        const val POSTPONED = "postponed"
        const val BAD = "bad"
        const val PROBLEM = "problem"
        const val CHECKED_AT = "checked_at"
        const val INSTALLING = "installing"
        const val ASKED_FOR_INSTALLS_AT = "asked_for_installs_at"
        const val REMINDED_VERSION = "reminded_version"
        const val TOLD_TOO_OLD_FOR = "told_too_old_for"
    }
}
