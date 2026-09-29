package com.tonypine.jobsearchhub.core

import kotlinx.serialization.Serializable

@Serializable
data class Company(
    val id: String,
    val name: String,
    val domain: String? = null,
    val summary: String? = null,
    val headquartersCountry: String? = null,
    val careersUrl: String? = null,
)

@Serializable
data class Person(val name: String, val roleTitle: String? = null, val relevance: String = "")

/** Someone the owner knows who can open doors at the company without working there. */
@Serializable
data class WarmPath(val name: String, val howKnown: String? = null, val preferredChannel: String? = null, val note: String? = null)

/** What the hub knows about a company. */
@Serializable
data class CompanyDossier(
    val company: Company,
    val people: List<Person> = emptyList(),
    val connections: List<Connection>? = null,
    val warmPaths: List<WarmPath>? = null,
)

@Serializable
data class PipelinePhase(val id: String, val name: String, val isClosed: Boolean = false)

@Serializable
data class Application(val companyId: String? = null, val jobId: String? = null, val phaseId: String, val notes: String? = null, val closedReason: String? = null)

@Serializable
data class PipelineCard(val application: Application, val jobTitle: String? = null, val companyName: String? = null)

@Serializable
data class PipelineBoard(val phases: List<PipelinePhase>, val cards: List<PipelineCard>) {
    /** The company's cards, each with its phase's name. */
    fun findCards(companyId: String): List<Pair<PipelineCard, String>> =
        cards.filter { it.application.companyId == companyId }
            .map { card -> card to (phases.firstOrNull { it.id == card.application.phaseId }?.name ?: "") }
}

/** Work asked of the Mac, which is the only place agents run. */
@Serializable
data class TaskRequest(val id: String, val kind: String, val status: String, val companyId: String? = null, val input: String? = null)

@Serializable
data class QueueTaskRequest(val kind: String, val companyId: String? = null, val company: String? = null)
