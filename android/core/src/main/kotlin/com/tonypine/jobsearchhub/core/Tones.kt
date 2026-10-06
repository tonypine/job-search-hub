package com.tonypine.jobsearchhub.core

/**
 * What a color means. Every state the clients color maps to one tone here, the
 * same table as the Mac's `JobSearchHubCore`, so the two can't drift.
 */
enum class Tone { ACCENT, POSITIVE, CAUTION, NEGATIVE, NEUTRAL }

/** A brief's verdict on the job. */
enum class Match(val word: String, val tone: Tone) {
    STRONG("Strong", Tone.POSITIVE),
    POSSIBLE("Possible", Tone.ACCENT),
    STRETCH("Stretch", Tone.CAUTION),
    MISMATCH("Mismatch", Tone.NEUTRAL),
    ;

    /** The verdict as a header reads it: "Strong match", "Mismatch". */
    val label: String get() = if (this == MISMATCH) word else "$word match"

    companion object {
        /** The hub's `match`; one it doesn't know reads as a mismatch. */
        fun of(match: String): Match = entries.firstOrNull { it.name.equals(match, ignoreCase = true) } ?: MISMATCH
    }
}

/** Whether a rule rules the owner out: the fit and the screen-out checks. */
enum class Screen(val word: String, val tone: Tone) {
    PASSES("Passes", Tone.POSITIVE),
    UNCLEAR("Unclear", Tone.CAUTION),
    FAILS("Fails", Tone.NEGATIVE),
    ;

    /** The job's screen as a chip reads it: "Passes screen". */
    val label: String get() = when (this) {
        PASSES -> "Passes screen"
        UNCLEAR -> "Screen unclear"
        FAILS -> "Fails screen"
    }

    companion object {
        /** A fit's level: good, unclear or poor. */
        fun ofLevel(level: String): Screen = when (level) {
            "good" -> PASSES
            "poor" -> FAILS
            else -> UNCLEAR
        }

        /** A check's verdict: yes, unclear or no; null is information no rule judges. */
        fun ofVerdict(verdict: String?): Screen? = when (verdict) {
            null -> null
            "yes" -> PASSES
            "no" -> FAILS
            else -> UNCLEAR
        }
    }
}

/** When a pipeline card's follow-up falls. */
enum class FollowUpDue(val tone: Tone) {
    OVERDUE(Tone.NEGATIVE),
    TODAY(Tone.CAUTION),
    LATER(Tone.NEUTRAL),
}

/** A job or card taken out: skipped, or closed with an outcome. */
enum class SetAside(val word: String) {
    SKIPPED("Skipped"),
    CLOSED("Closed"),
    ;

    val tone: Tone get() = Tone.NEUTRAL
}

/** What a Claude session is doing. */
enum class SessionState(val tone: Tone) {
    WORKING(Tone.ACCENT),
    WAITING_FOR_YOU(Tone.CAUTION),
    IDLE(Tone.POSITIVE),
}

/** One row of a job's Screen: a rule, its verdict, why, and the posting's words when there are some. */
data class ScreenRow(val name: String, val screen: Screen?, val reason: String, val evidence: String? = null) {
    val tone: Tone get() = screen?.tone ?: Tone.NEUTRAL
}

/**
 * The job's Screen: the screen-out answers, then the fit checks they don't
 * already answer. The hub answers some screen-out checks from a fit check under
 * another name, with the check's verdict and reason, so those show once.
 */
fun JobDetails.screenRows(): List<ScreenRow> {
    val answers = screenOut.map { ScreenRow(it.name, Screen.ofVerdict(it.verdict), it.answer, it.evidence) }
    val checks = fit.checks
        .filter { check -> screenOut.none { it.verdict == check.verdict && it.answer == check.reason } }
        .map { ScreenRow(it.name, Screen.ofVerdict(it.verdict), it.reason) }
    return answers + checks
}
