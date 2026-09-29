package com.tonypine.jobsearchhub

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.tonypine.jobsearchhub.core.HubUpdate
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.JobListItem
import com.tonypine.jobsearchhub.core.JobsOrder
import com.tonypine.jobsearchhub.core.Pairing
import com.tonypine.jobsearchhub.core.PairingLink
import com.tonypine.jobsearchhub.data.HubClient
import com.tonypine.jobsearchhub.data.HubException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class HubState(
    val pairing: Pairing? = null,
    val updates: List<HubUpdate> = emptyList(),
    val jobs: List<JobListItem> = emptyList(),
    /** How many open jobs the hub holds; the list is its newest page. */
    val openJobCount: Int = 0,
    val includesUnclear: Boolean = false,
    val isLoading: Boolean = false,
    val error: String? = null,
) {
    val shownJobs: List<JobListItem> get() = JobsOrder.pick(jobs, includesUnclear)
}

/** The phone's view of the hub: its pairing, the updates and the jobs. */
class HubViewModel(application: Application) : AndroidViewModel(application) {
    private val store = (application as HubApp).pairingStore
    private val mutableState = MutableStateFlow(HubState(pairing = store.load()))
    val state: StateFlow<HubState> = mutableState.asStateFlow()

    private val client: HubClient? get() = state.value.pairing?.let { HubClient(it) }

    init {
        refresh()
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
                mutableState.update { it.copy(updates = updates, jobs = jobs.jobs, openJobCount = jobs.total, isLoading = false) }
            } catch (error: HubException) {
                mutableState.update { it.copy(isLoading = false, error = error.message) }
            }
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
