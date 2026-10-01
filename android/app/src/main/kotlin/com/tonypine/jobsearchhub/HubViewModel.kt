package com.tonypine.jobsearchhub

import android.app.Application
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.tonypine.jobsearchhub.core.CompanyDossier
import com.tonypine.jobsearchhub.core.HubUpdate
import com.tonypine.jobsearchhub.core.DecisionQueueItem
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.JobListItem
import com.tonypine.jobsearchhub.core.JobsOrder
import com.tonypine.jobsearchhub.core.Pairing
import com.tonypine.jobsearchhub.core.PairingLink
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.data.HubClient
import com.tonypine.jobsearchhub.data.HubException
import com.tonypine.jobsearchhub.push.UpdateNotifications
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException

/** A company as the phone briefs it before an interview. */
data class CompanyBrief(
    val dossier: CompanyDossier,
    val openJobs: List<JobListItem>,
    val cards: List<Pair<PipelineCard, String>>,
)

data class HubState(
    val pairing: Pairing? = null,
    val updates: List<HubUpdate> = emptyList(),
    val jobs: List<JobListItem> = emptyList(),
    /** The briefed jobs waiting for a decision, best match first. */
    val decisionQueue: List<DecisionQueueItem> = emptyList(),
    /** How many open jobs the hub holds; the list is its newest page. */
    val openJobCount: Int = 0,
    val includesUnclear: Boolean = false,
    val isLoading: Boolean = false,
    val error: String? = null,
) {
    val shownJobs: List<JobListItem> get() = JobsOrder.pick(jobs, includesUnclear)
}

/** What a tapped notification opens: its job, or else its company. */
data class NotificationTarget(val jobId: String?, val companyId: String?)

/** The phone's view of the hub: its pairing, the updates and the jobs. */
class HubViewModel(application: Application) : AndroidViewModel(application) {
    private val app = application as HubApp
    private val store = app.pairingStore
    private val mutableState = MutableStateFlow(HubState(pairing = store.load()))
    val state: StateFlow<HubState> = mutableState.asStateFlow()
    private val mutableNotificationTarget = MutableStateFlow<NotificationTarget?>(null)
    val notificationTarget: StateFlow<NotificationTarget?> = mutableNotificationTarget.asStateFlow()

    private val client: HubClient? get() = state.value.pairing?.let { HubClient(it) }

    init {
        refresh()
        registerForPushes()
        viewModelScope.launch { app.pushes.collect { refresh() } }
    }

    /** Sends the hub this app's FCM token. Done at each start, so a token rotated while the phone was unpaired or offline still arrives. */
    private fun registerForPushes() {
        val client = client ?: return
        if (!UpdateNotifications.isAvailable(app)) {
            return
        }
        viewModelScope.launch {
            try {
                client.setPushToken(UpdateNotifications.getPushToken())
            } catch (error: HubException) {
                Log.w("HubViewModel", "Couldn't register for pushes: ${error.message}")
            } catch (error: IOException) {
                Log.w("HubViewModel", "FCM gave no token: ${error.message}")
            }
        }
    }

    /** Opens what a tapped notification is about, with the lists read again. */
    fun openNotification(target: NotificationTarget) {
        if (target.jobId == null && target.companyId == null) {
            return
        }
        mutableNotificationTarget.value = target
        refresh()
    }

    fun clearNotificationTarget() {
        mutableNotificationTarget.value = null
    }

    /** Pairs from a scanned or pasted link; says why a link isn't one. */
    fun pair(link: String): Boolean {
        val pairing = PairingLink.parse(link) ?: run {
            mutableState.update { it.copy(error = "That isn't a pairing link. Use the QR code in the Mac app's Settings › Phones.") }
            return false
        }
        store.save(pairing)
        mutableState.update { HubState(pairing = pairing) }
        refresh()
        registerForPushes()
        return true
    }

    fun unpair() {
        store.forget()
        mutableState.update { HubState() }
    }

    fun setIncludesUnclear(includes: Boolean) {
        mutableState.update { it.copy(includesUnclear = includes) }
    }

    fun refresh() {
        val client = client ?: return
        mutableState.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            try {
                val updates = client.getUpdates().updates
                val jobs = client.getJobs()
                val queue = client.getDecisionQueue().items
                mutableState.update { it.copy(updates = updates, jobs = jobs.jobs, openJobCount = jobs.total, decisionQueue = queue, isLoading = false) }
            } catch (error: HubException) {
                mutableState.update { it.copy(isLoading = false, error = error.message) }
            }
        }
    }

    /** Everything a company's brief shows, read together. */
    suspend fun loadCompanyBrief(id: String): Result<CompanyBrief> {
        val client = client ?: return Result.failure(HubException("Not paired."))
        return try {
            val dossier = client.getCompany(id)
            val jobs = client.getCompanyJobs(id).jobs
            val cards = client.getPipeline().findCards(id)
            Result.success(CompanyBrief(dossier, jobs, cards))
        } catch (error: HubException) {
            Result.failure(error)
        }
    }

    /** Asks the Mac for work; its progress and result arrive as updates. */
    suspend fun askTheMac(request: QueueTaskRequest): Result<Unit> {
        val client = client ?: return Result.failure(HubException("Not paired."))
        return try {
            client.queueTask(request)
            refresh()
            Result.success(Unit)
        } catch (error: HubException) {
            Result.failure(error)
        }
    }

    /**
     * Records the decision on the job, reads the lists again, and returns the
     * job after it in the decision queue, if any.
     */
    suspend fun decideJob(id: String, decision: String, reason: String = ""): Result<String?> {
        val client = client ?: return Result.failure(HubException("Not paired."))
        val queue = state.value.decisionQueue
        val next = queue.getOrNull(queue.indexOfFirst { it.job.id == id } + 1)?.job?.id
        return try {
            client.decideJob(id, decision, reason)
            refresh()
            Result.success(next)
        } catch (error: HubException) {
            Result.failure(error)
        }
    }

    suspend fun loadJob(id: String): Result<JobDetails> {
        val client = client ?: return Result.failure(HubException("Not paired."))
        return try {
            Result.success(client.getJob(id))
        } catch (error: HubException) {
            Result.failure(error)
        }
    }
}
