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

/** Work asked of the Mac, which is the only place agents run. */
@Serializable
data class TaskRequest(val id: String, val kind: String, val status: String, val companyId: String? = null, val jobId: String? = null, val input: String? = null)

@Serializable
data class QueueTaskRequest(
    val kind: String,
    val companyId: String? = null,
    val company: String? = null,
    /** A fix_job's job and the note on what's wrong with it. */
    val jobId: String? = null,
    val note: String? = null,
)

@Serializable
data class SetPushTokenRequest(val token: String)
