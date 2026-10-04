package com.tonypine.jobsearchhub.core

import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class PipelineTest {
    private val zone = ZoneId.of("America/Sao_Paulo")
    private val now = Instant.parse("2026-10-04T15:00:00Z")

    private fun card(id: String, phaseId: String = "applied", jobId: String? = null, companyId: String? = null, entered: String = "2026-09-25T12:00:00Z", due: String? = null) =
        PipelineCard(Application(id = id, jobId = jobId, companyId = companyId, phaseId = phaseId, phaseEnteredAt = entered), jobTitle = "Engineer $id", followUpDueAt = due)

    @Test
    fun theHubsBoardReads() {
        val board = hubJson.decodeFromString<PipelineBoard>(
            """{"phases":[{"id":"p2","name":"Screening","position":2},{"id":"p1","name":"Applied","position":1,"follow_up_days":7},
            {"id":"p3","name":"Closed","position":3,"is_closed":true}],
            "cards":[{"application":{"id":"a1","job_id":"j1","company_id":"c1","phase_id":"p1","phase_entered_at":"2026-09-25T11:59:59.123456-03:00",
            "last_followed_up_at":"2026-09-28T09:00:00Z","contacted_at":"2026-09-30T09:00:00Z","created_at":"2026-09-25T12:00:00Z"},
            "job_title":"Engineer","company_name":"Acme","follow_up_due_at":"2026-10-02T12:00:00Z","unseen_updates":1}]}""",
        )
        assertEquals(listOf("Applied", "Screening", "Closed"), board.orderedPhases.map { it.name })
        assertEquals(7, board.orderedPhases.first().followUpDays)
        val card = board.cards.single()
        assertEquals("a1", card.id)
        assertEquals(true, card.isHeardBack)
        assertEquals("Applied", board.phaseOf(card)?.name)
        assertEquals(9, card.daysInPhase(now))
    }

    @Test
    fun aFollowUpIsOverdueDueTodayOrDueLaterByCalendarDay() {
        // 15:00 UTC is noon in São Paulo; 02:00 UTC the next day is still the 4th there.
        assertEquals(FollowUpStatus(FollowUpDue.OVERDUE, 2), card("a", due = "2026-10-02T23:00:00Z").followUpStatus(now, zone))
        assertEquals(FollowUpStatus(FollowUpDue.TODAY, 0), card("a", due = "2026-10-05T02:00:00Z").followUpStatus(now, zone))
        assertEquals(FollowUpStatus(FollowUpDue.LATER, 4), card("a", due = "2026-10-08T15:00:00Z").followUpStatus(now, zone))
        assertNull(card("a").followUpStatus(now, zone))
        assertEquals(
            listOf("Overdue 1 day", "Overdue 2 days", "Due today", "Due tomorrow", "Due in 4 days"),
            listOf(
                FollowUpStatus(FollowUpDue.OVERDUE, 1), FollowUpStatus(FollowUpDue.OVERDUE, 2), FollowUpStatus(FollowUpDue.TODAY, 0),
                FollowUpStatus(FollowUpDue.LATER, 1), FollowUpStatus(FollowUpDue.LATER, 4),
            ).map { it.label },
        )
    }

    @Test
    fun theFollowUpsDueAreOverdueOrTodayTheLongestOverdueFirst() {
        val board = PipelineBoard(
            phases = listOf(PipelinePhase("applied", "Applied")),
            cards = listOf(
                card("today", due = "2026-10-04T20:00:00Z"),
                card("later", due = "2026-10-09T12:00:00Z"),
                card("one", due = "2026-10-03T12:00:00Z"),
                card("five", due = "2026-09-29T12:00:00Z"),
                card("none"),
            ),
        )
        assertEquals(listOf("five", "one", "today"), board.dueFollowUps(now, zone).map { it.first.id })
    }

    @Test
    fun aPhasesCardsAreNewestInThePhaseFirst() {
        val board = PipelineBoard(
            phases = listOf(PipelinePhase("applied", "Applied"), PipelinePhase("screening", "Screening")),
            cards = listOf(
                card("old", entered = "2026-09-01T12:00:00Z"),
                card("other", phaseId = "screening"),
                card("new", entered = "2026-10-01T12:00:00-03:00"),
            ),
        )
        assertEquals(listOf("new", "old"), board.cardsIn("applied").map { it.id })
    }

    @Test
    fun aReminderFindsItsJobsCardOrElseItsCompanysCardWithNoJob() {
        val board = PipelineBoard(
            phases = listOf(PipelinePhase("applied", "Applied")),
            cards = listOf(card("job", jobId = "j1", companyId = "c1"), card("company", companyId = "c1"), card("elsewhere", jobId = "j2", companyId = "c2")),
        )
        assertEquals("job", board.findCard("j1", "c1")?.id)
        assertEquals("company", board.findCard(null, "c1")?.id)
        assertEquals("elsewhere", board.findCard("gone", "c2")?.id)
        assertNull(board.findCard(null, "c3"))
        assertNull(board.findCard(null, null))
    }

    @Test
    fun aMoveAndAFollowUpEncodeAsTheHubReadsThem() {
        assertEquals("""{"phase_id":"p3","closed_reason":"Took another offer"}""", hubJson.encodeToString(MoveApplicationRequest.serializer(), MoveApplicationRequest("p3", "Took another offer")))
        assertEquals("""{"note":"Wrote to the recruiter"}""", hubJson.encodeToString(FollowUpRequest.serializer(), FollowUpRequest("Wrote to the recruiter")))
    }
}
