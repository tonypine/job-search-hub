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

    val channel: NoticeChannel get() = NoticeChannel.of(kind)

    /** The notification's buttons: a reminder records or snoozes, a reply opens or is marked read. */
    val actions: List<NoticeAction> get() = when (channel) {
        // Followed up finds the card by its job or company; a reminder naming neither can only wait.
        NoticeChannel.FOLLOW_UPS -> listOfNotNull(NoticeAction.FOLLOWED_UP.takeIf { jobId != null || companyId != null }, NoticeAction.SNOOZE_A_DAY)
        NoticeChannel.REPLIES -> listOf(NoticeAction.OPEN, NoticeAction.MARK_AS_READ)
        NoticeChannel.MATCHES, NoticeChannel.HUB -> emptyList()
    }

    /** The update as a push's data, which `parse` reads back; intents and scheduled work carry it this way. */
    fun toData(): Map<String, String> = buildMap {
        put("update_id", updateId)
        put("title", title)
        body?.let { put("body", it) }
        jobId?.let { put("job_id", it) }
        companyId?.let { put("company_id", it) }
        kind?.let { put("kind", it) }
    }

    companion object {
        /** Reads a push's data; null when it isn't one of the hub's updates. */
        fun parse(data: Map<String, String>): PushedUpdate? {
            val updateId = data["update_id"]?.takeIf { it.isNotBlank() } ?: return null
            val title = data["title"]?.takeIf { it.isNotBlank() } ?: return null
            return PushedUpdate(updateId, title, data["body"], data["job_id"], data["company_id"], data["kind"])
        }
    }
}
