package com.tonypine.jobsearchhub.data

import com.tonypine.jobsearchhub.core.CompanyDossier
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.DecisionQueueResponse
import com.tonypine.jobsearchhub.core.JobDecision
import com.tonypine.jobsearchhub.core.JobDecisionRequest
import com.tonypine.jobsearchhub.core.JobsResponse
import com.tonypine.jobsearchhub.core.Pairing
import com.tonypine.jobsearchhub.core.PipelineBoard
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.core.SetPushTokenRequest
import com.tonypine.jobsearchhub.core.TaskRequest
import com.tonypine.jobsearchhub.core.UpdatesResponse
import com.tonypine.jobsearchhub.core.hubJson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit

/** Why a call to the hub failed, in words the app can show. */
class HubException(message: String, val isRefused: Boolean = false) : IOException(message)

/** Reads the hub's REST API with the phone's device token. */
class HubClient(
    private val pairing: Pairing,
    private val http: OkHttpClient = OkHttpClient.Builder().callTimeout(20, TimeUnit.SECONDS).build(),
) {
    suspend fun getUpdates(): UpdatesResponse = get("/v1/updates?limit=100")

    suspend fun getJobs(): JobsResponse = get("/v1/jobs?status=open&limit=500")

    suspend fun getJob(id: String): JobDetails = get("/v1/jobs/$id")

    suspend fun getCompany(id: String): CompanyDossier = get("/v1/companies/$id")

    suspend fun getCompanyJobs(id: String): JobsResponse = get("/v1/jobs?company_id=$id&status=open&limit=200")

    suspend fun getPipeline(): PipelineBoard = get("/v1/pipeline")

    /** Asks the Mac to find a company's jobs, or to research a company by name or link. */
    suspend fun queueTask(request: QueueTaskRequest): TaskRequest = send("/v1/tasks", hubJson.encodeToString(request))

    /** The briefed jobs waiting for a decision, best match first. */
    suspend fun getDecisionQueue(): DecisionQueueResponse = get("/v1/decision-queue")

    /** Records a decision: pursue puts the job on the pipeline, skip dismisses it with the reason, later only records. */
    suspend fun decideJob(id: String, decision: String, reason: String = ""): JobDecision =
        send("/v1/jobs/$id/decision", hubJson.encodeToString(JobDecisionRequest(decision, reason.trim())))

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
            .method(method, json?.toRequestBody("application/json".toMediaType()))
            .build()
        try {
            http.newCall(request).execute().use { response ->
                when {
                    response.code == 401 || response.code == 403 ->
                        throw HubException("The hub refused this phone's token; pair it again from the Mac.", isRefused = true)
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
}
