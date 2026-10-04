package com.tonypine.jobsearchhub.core

/** An update as the hub pushes it; the phone builds the notification from it. */
data class PushedUpdate(
    val updateId: String,
    val title: String,
    val body: String?,
    val jobId: String?,
    val companyId: String?,
    val kind: String? = null,
) {
    /** A follow-up reminder, which opens its card on the pipeline. */
    val isFollowUp: Boolean get() = kind == HubUpdate.FOLLOW_UP_DUE

    companion object {
        /** Reads a push's data; null when it isn't one of the hub's updates. */
        fun parse(data: Map<String, String>): PushedUpdate? {
            val updateId = data["update_id"]?.takeIf { it.isNotBlank() } ?: return null
            val title = data["title"]?.takeIf { it.isNotBlank() } ?: return null
            return PushedUpdate(updateId, title, data["body"], data["job_id"], data["company_id"], data["kind"])
        }
    }
}
