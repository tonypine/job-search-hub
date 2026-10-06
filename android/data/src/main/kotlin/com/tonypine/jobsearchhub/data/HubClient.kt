package com.tonypine.jobsearchhub.data

import com.tonypine.jobsearchhub.core.ClearedJobDecision
import com.tonypine.jobsearchhub.core.CompanyDossier
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.DecisionQueueResponse
import com.tonypine.jobsearchhub.core.FollowUpRequest
import com.tonypine.jobsearchhub.core.JobDecision
import com.tonypine.jobsearchhub.core.JobDecisionRequest
import com.tonypine.jobsearchhub.core.JobsResponse
import com.tonypine.jobsearchhub.core.MarkUpdatesSeenRequest
import com.tonypine.jobsearchhub.core.MoveApplicationRequest
import com.tonypine.jobsearchhub.core.Pairing
import com.tonypine.jobsearchhub.core.PipelineBoard
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.core.RecruitersResponse
import com.tonypine.jobsearchhub.core.SetPushTokenRequest
import com.tonypine.jobsearchhub.core.TaskRequest
import com.tonypine.jobsearchhub.core.UpdatesResponse
import com.tonypine.jobsearchhub.core.hubJson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit

/**
 * Why a call to the hub failed, in words the app can show. [isUpgradeRequired] says the hub no longer serves this
 * version of the app, and the message is the hub's own, saying what to install.
 */
class HubException(message: String, val isRefused: Boolean = false, val isUpgradeRequired: Boolean = false) : IOException(message)

/**
 * Reads the hub's REST API with the phone's device token, naming the app's [appVersion] on every call, so the hub
 * keeps it beside the phone's name and can turn away a version it no longer serves.
 */
class HubClient(
    private val pairing: Pairing,
    private val appVersion: String,
    private val http: OkHttpClient = OkHttpClient.Builder().callTimeout(20, TimeUnit.SECONDS).build(),
) {
    suspend fun getUpdates(): UpdatesResponse = get("/v1/updates?limit=100")

    /** Asks the hub for the least it serves, so a check in the background learns whether it still serves this version. */
    suspend fun checkServed() {
        fetch("GET", "/v1/updates?limit=1", null)
    }

    /** Every open job, read a page at a time until the hub's total, so the oldest ones are not left out. */
    suspend fun getJobs(): JobsResponse {
        val first = getOpenJobsPage(offset = 0)
        val jobs = first.jobs.toMutableList()
        while (jobs.size < first.total) {
            val page = getOpenJobsPage(offset = jobs.size)
            if (page.jobs.isEmpty()) break
            jobs += page.jobs
        }
        return first.copy(jobs = jobs)
    }

    private suspend fun getOpenJobsPage(offset: Int): JobsResponse =
        get("/v1/jobs?status=open&limit=$JOBS_PAGE_SIZE" + if (offset > 0) "&offset=$offset" else "")

    suspend fun getJob(id: String): JobDetails = get("/v1/jobs/$id")

    suspend fun getCompany(id: String): CompanyDossier = get("/v1/companies/$id")

    suspend fun getCompanyJobs(id: String): JobsResponse = get("/v1/jobs?company_id=$id&status=open&limit=200")

    suspend fun getPipeline(): PipelineBoard = get("/v1/pipeline")

    /** Notes that the owner followed up on the application now, which restarts its phase's follow-up count. */
    suspend fun recordFollowUp(applicationId: String, note: String) {
        fetch("POST", "/v1/applications/$applicationId/follow-ups", hubJson.encodeToString(FollowUpRequest(note.trim())))
    }

    /** Moves the application to the phase; a closed phase keeps the reason it ended. */
    suspend fun moveApplication(applicationId: String, phaseId: String, closedReason: String = "") {
        fetch("PATCH", "/v1/applications/$applicationId", hubJson.encodeToString(MoveApplicationRequest(phaseId, closedReason.trim())))
    }

    /** The conversations recruiters started on LinkedIn, the latest first. */
    suspend fun getRecruiters(): RecruitersResponse = get("/v1/recruiters")

    suspend fun markUpdatesSeen(ids: List<String>) {
        fetch("POST", "/v1/updates/seen", hubJson.encodeToString(MarkUpdatesSeenRequest(ids)))
    }

    /** Asks the Mac to find a company's jobs, or to research a company by name or link. */
    suspend fun queueTask(request: QueueTaskRequest): TaskRequest = send("/v1/tasks", hubJson.encodeToString(request))

    /** The briefed jobs waiting for a decision, best match first. */
    suspend fun getDecisionQueue(): DecisionQueueResponse = get("/v1/decision-queue")

    /** Records a decision: pursue puts the job on the pipeline, skip dismisses it with the reason, later only records. */
    suspend fun decideJob(id: String, decision: String, reason: String = ""): JobDecision =
        send("/v1/jobs/$id/decision", hubJson.encodeToString(JobDecisionRequest(decision, reason.trim())))

    /**
     * Takes back the decision on a job, which leaves it undecided: a skipped job comes back to the jobs list, and a
     * pursued one leaves the pipeline unless its card was there before or changed since. A hub that answers with no
     * body says nothing about the card.
     */
    suspend fun clearJobDecision(id: String): ClearedJobDecision {
        val body = fetch("DELETE", "/v1/jobs/$id/decision", null)
        return if (body.isBlank()) ClearedJobDecision() else hubJson.decodeFromString(body)
    }

    /** Registers the token FCM gave this app, so the hub pushes its updates here. */
    suspend fun setPushToken(token: String) {
        fetch("PUT", "/v1/devices/me/push-token", hubJson.encodeToString(SetPushTokenRequest(token)))
    }

    private suspend inline fun <reified T> get(path: String): T = hubJson.decodeFromString(fetch("GET", path, null))

    private suspend inline fun <reified T> send(path: String, json: String): T = hubJson.decodeFromString(fetch("POST", path, json))

    private suspend fun fetch(method: String, path: String, json: String?): String = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url(pairing.hubUrl + path)
            .header("Authorization", "Bearer ${pairing.token}")
            .header(CLIENT_HEADER, "android/$appVersion")
            .method(method, json?.toRequestBody("application/json".toMediaType()))
            .build()
        try {
            http.newCall(request).execute().use { response ->
                when {
                    response.code == 401 || response.code == 403 ->
                        throw HubException("The hub refused this phone's token; pair it again from the Mac.", isRefused = true)
                    response.code == UPGRADE_REQUIRED ->
                        throw HubException(
                            readError(response.body.string()) ?: "The hub no longer serves this version of the app. Install a newer one.",
                            isUpgradeRequired = true,
                        )
                    !response.isSuccessful -> throw HubException("The hub answered ${response.code}.")
                    else -> response.body.string()
                }
            }
        } catch (error: HubException) {
            throw error
        } catch (error: IOException) {
            throw HubException("Can't reach the hub at ${pairing.hubUrl}: ${error.message}")
        }
    }

    /** The message of the hub's `{"error": …}` answer, or null for any other body. */
    private fun readError(body: String): String? = runCatching {
        hubJson.parseToJsonElement(body).jsonObject["error"]?.jsonPrimitive?.content
    }.getOrNull()?.takeIf { it.isNotBlank() }

    private companion object {
        /** The most jobs the hub returns in one page. */
        const val JOBS_PAGE_SIZE = 500

        /** Names the app and its version, as `android/0.1.252`. */
        const val CLIENT_HEADER = "X-Hub-Client"

        /** The hub's answer to a version of the app it no longer serves, with a message saying what to install. */
        const val UPGRADE_REQUIRED = 426
    }
}
