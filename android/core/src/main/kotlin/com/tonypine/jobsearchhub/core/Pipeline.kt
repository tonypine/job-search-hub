package com.tonypine.jobsearchhub.core

import kotlinx.serialization.Serializable
import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.temporal.ChronoUnit

/** A phase of the pipeline; a closed one is where finished applications rest. */
@Serializable
data class PipelinePhase(
    val id: String,
    val name: String,
    val position: Int = 0,
    val isClosed: Boolean = false,
    /** Days a card may sit here, since it entered or was last followed up, before a follow-up is due; null never falls due. */
    val followUpDays: Int? = null,
)

@Serializable
data class Application(
    val id: String,
    val companyId: String? = null,
    val jobId: String? = null,
    val phaseId: String,
    val notes: String? = null,
    val closedReason: String? = null,
    val phaseEnteredAt: String,
    val lastFollowedUpAt: String? = null,
    /** When a person at the company first wrote back; null until someone does. */
    val contactedAt: String? = null,
)

/** One card of the board: an application with the job and company it is for. */
@Serializable
data class PipelineCard(
    val application: Application,
    val jobTitle: String? = null,
    val companyName: String? = null,
    /** When the card's phase wants a follow-up; null when it asks for none. */
    val followUpDueAt: String? = null,
    val unseenUpdates: Int = 0,
) {
    val id: String get() = application.id

    /** The job's title, or the company's name for an application with no job. */
    val title: String get() = jobTitle ?: companyName ?: "Untitled"

    val isHeardBack: Boolean get() = application.contactedAt != null

    /** Where the card stands on its follow-up, by calendar day in zone. */
    fun followUpStatus(now: Instant, zone: ZoneId): FollowUpStatus? {
        val dueAt = parseInstant(followUpDueAt) ?: return null
        val days = ChronoUnit.DAYS.between(now.atZone(zone).toLocalDate(), dueAt.atZone(zone).toLocalDate()).toInt()
        return when {
            days < 0 -> FollowUpStatus(FollowUpDue.OVERDUE, -days)
            days == 0 -> FollowUpStatus(FollowUpDue.TODAY, 0)
            else -> FollowUpStatus(FollowUpDue.LATER, days)
        }
    }

    /** Whole days since the card entered its phase. */
    fun daysInPhase(now: Instant): Int = parseInstant(application.phaseEnteredAt)?.let { Duration.between(it, now).toDays().toInt().coerceAtLeast(0) } ?: 0
}

/** A card's follow-up: overdue by `days`, due today, or due in `days`. */
data class FollowUpStatus(val due: FollowUpDue, val days: Int) {
    val tone: Tone get() = due.tone

    /** Whether it needs doing now: overdue or due today. */
    val isDue: Boolean get() = due != FollowUpDue.LATER

    val label: String get() = when (due) {
        FollowUpDue.OVERDUE -> if (days == 1) "Overdue 1 day" else "Overdue $days days"
        FollowUpDue.TODAY -> "Due today"
        FollowUpDue.LATER -> if (days == 1) "Due tomorrow" else "Due in $days days"
    }
}

/** The board as the phone shows it: phases in order, and each phase's cards newest in the phase first. */
@Serializable
data class PipelineBoard(val phases: List<PipelinePhase>, val cards: List<PipelineCard>) {
    val orderedPhases: List<PipelinePhase> get() = phases.sortedBy { it.position }

    fun cardsIn(phaseId: String): List<PipelineCard> =
        cards.filter { it.application.phaseId == phaseId }.sortedByDescending { parseInstant(it.application.phaseEnteredAt) }

    fun phaseOf(card: PipelineCard): PipelinePhase? = phases.firstOrNull { it.id == card.application.phaseId }

    /** The company's cards, each with its phase's name. */
    fun findCards(companyId: String): List<Pair<PipelineCard, String>> =
        cards.filter { it.application.companyId == companyId }.map { card -> card to (phaseOf(card)?.name ?: "") }

    /** The cards whose follow-up is overdue or due today, the longest overdue first. */
    fun dueFollowUps(now: Instant, zone: ZoneId): List<Pair<PipelineCard, FollowUpStatus>> =
        cards.mapNotNull { card -> card.followUpStatus(now, zone)?.takeIf { it.isDue }?.let { card to it } }
            .sortedByDescending { (_, status) -> if (status.due == FollowUpDue.OVERDUE) status.days else 0 }

    /**
     * The card an update about a job or a company is for: the job's card, or
     * else the company's, its card with no job first, as a reminder about a
     * company-only application names no job.
     */
    fun findCard(jobId: String?, companyId: String?): PipelineCard? {
        jobId?.let { id -> cards.firstOrNull { it.application.jobId == id }?.let { return it } }
        companyId ?: return null
        return cards.filter { it.application.companyId == companyId }.minByOrNull { if (it.application.jobId == null) 0 else 1 }
    }

    /**
     * Whether a reminder about the job or company still asks for a follow-up:
     * false once its card is gone, or followed up so the next one isn't due.
     * A snoozed reminder that no longer does stays away.
     */
    fun wantsFollowUp(jobId: String?, companyId: String?, now: Instant, zone: ZoneId): Boolean =
        findCard(jobId, companyId)?.followUpStatus(now, zone)?.isDue == true
}

/** Moves an application to a phase; a closed phase keeps the reason it ended. */
@Serializable
data class MoveApplicationRequest(val phaseId: String, val closedReason: String = "")

@Serializable
data class FollowUpRequest(val note: String)

/** The hub's RFC 3339 timestamp, in UTC or with an offset; null when there's none or it doesn't read. */
fun parseInstant(timestamp: String?): Instant? = timestamp?.let { runCatching { OffsetDateTime.parse(it).toInstant() }.getOrNull() }
