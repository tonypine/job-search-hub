package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** The state→tone table of docs/design/ui-redesign.md, row by row. */
class TonesTest {
    @Test
    fun aMatchIsPositiveAccentCautionOrNeutral() {
        assertEquals(
            listOf(Tone.POSITIVE, Tone.ACCENT, Tone.CAUTION, Tone.NEUTRAL),
            listOf("strong", "possible", "stretch", "mismatch").map { Match.of(it).tone },
        )
        assertEquals(Match.MISMATCH, Match.of("something new"))
        assertEquals(listOf("Strong match", "Possible match", "Stretch match", "Mismatch"), Match.entries.map { it.label })
    }

    @Test
    fun aScreenIsPositiveCautionOrNegative() {
        assertEquals(listOf(Tone.POSITIVE, Tone.CAUTION, Tone.NEGATIVE), listOf("good", "unclear", "poor").map { Screen.ofLevel(it).tone })
        assertEquals(listOf(Screen.PASSES, Screen.UNCLEAR, Screen.FAILS), listOf("yes", "unclear", "no").map { Screen.ofVerdict(it) })
        assertNull(Screen.ofVerdict(null))
        assertEquals("Passes screen", Screen.PASSES.label)
    }

    @Test
    fun aFollowUpIsNegativeCautionOrNeutral() {
        assertEquals(listOf(Tone.NEGATIVE, Tone.CAUTION, Tone.NEUTRAL), FollowUpDue.entries.map { it.tone })
    }

    @Test
    fun skippedAndClosedAreNeutral() {
        assertEquals(listOf(Tone.NEUTRAL, Tone.NEUTRAL), SetAside.entries.map { it.tone })
        assertEquals("Skipped", SetAside.SKIPPED.word)
    }

    @Test
    fun aSessionIsAccentCautionOrPositive() {
        assertEquals(listOf(Tone.ACCENT, Tone.CAUTION, Tone.POSITIVE), SessionState.entries.map { it.tone })
    }

    @Test
    fun theScreenShowsAFitCheckOnceWhenAScreenOutAnswerRepeatsIt() {
        val details = JobDetails(
            job = Job(id = "j1", source = "x", title = "Engineer", url = "https://x/j1", firstSeenAt = "2026-10-01"),
            fit = JobFit(
                "unclear",
                listOf(FitCheck("Where they hire", "yes", "Americas"), FitCheck("Pay", "yes", "About 31k take-home")),
            ),
            screenOut = listOf(
                ScreenOutAnswer("Hires from Brazil", "yes", "Americas", evidence = "Remote in the Americas"),
                ScreenOutAnswer("Experience", "unclear", "asks for 6 years; you have 5"),
                ScreenOutAnswer("Contract", null, "PJ (contractor)"),
            ),
        )
        assertEquals(
            listOf(
                ScreenRow("Hires from Brazil", Screen.PASSES, "Americas", "Remote in the Americas"),
                ScreenRow("Experience", Screen.UNCLEAR, "asks for 6 years; you have 5"),
                ScreenRow("Contract", null, "PJ (contractor)"),
                ScreenRow("Pay", Screen.PASSES, "About 31k take-home"),
            ),
            details.screenRows(),
        )
        assertEquals(Tone.NEUTRAL, details.screenRows()[2].tone)
    }
}
