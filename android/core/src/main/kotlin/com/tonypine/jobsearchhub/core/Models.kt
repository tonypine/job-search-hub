package com.tonypine.jobsearchhub.core

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNamingStrategy

/** How the hub's JSON reads: snake_case keys, and fields the app doesn't use ignored. */
@OptIn(ExperimentalSerializationApi::class)
val hubJson = Json {
    namingStrategy = JsonNamingStrategy.SnakeCase
    ignoreUnknownKeys = true
    explicitNulls = false
}

/** Something the hub noticed or did, as the Updates page lists it. */
@Serializable
data class HubUpdate(
    val id: String,
    val kind: String,
    val title: String,
    val body: String = "",
    val sourceUrl: String? = null,
    val jobId: String? = null,
    val companyId: String? = null,
    val createdAt: String,
    val seenAt: String? = null,
    val jobTitle: String? = null,
    val companyName: String? = null,
) {
    companion object {
        /** A pipeline card's follow-up fell due. */
        const val FOLLOW_UP_DUE = "follow_up_due"

        /** A fresh job is a strong match. */
        const val FRESH_MATCH = "fresh_match"
    }
}

@Serializable
data class UpdatesResponse(val updates: List<HubUpdate>, val unseenCount: Int = 0)

@Serializable
data class Job(
    val id: String,
    val companyId: String? = null,
    val source: String,
    val title: String,
    val location: String? = null,
    val workplaceType: String? = null,
    val url: String,
    val description: String? = null,
    val employmentType: String? = null,
    val firstSeenAt: String,
)

@Serializable
data class FitCheck(val name: String, val verdict: String, val reason: String)

@Serializable
data class JobFit(val level: String, val checks: List<FitCheck> = emptyList())

@Serializable
data class JobListItem(
    val job: Job,
    val companyName: String? = null,
    val fit: JobFit,
    val unseenUpdates: Int = 0,
)

@Serializable
data class JobsResponse(val jobs: List<JobListItem>, val total: Int)

@Serializable
data class JobFactEntry(val key: String, val title: String, val value: JsonElement)

@Serializable
data class LabelledJobFacts(val entries: List<JobFactEntry> = emptyList())

/** One of the owner's LinkedIn connections who works at the job's company. */
@Serializable
data class Connection(val firstName: String = "", val lastName: String = "", val position: String? = null, val messageCount: Int = 0)

/** Everything the hub knows about one job. */
@Serializable
data class JobDetails(
    val job: Job,
    val companyName: String? = null,
    val fit: JobFit,
    val facts: LabelledJobFacts? = null,
    val connections: List<Connection>? = null,
    val brief: JobBrief? = null,
    val screenOut: List<ScreenOutAnswer> = emptyList(),
    val decision: JobDecision? = null,
)
