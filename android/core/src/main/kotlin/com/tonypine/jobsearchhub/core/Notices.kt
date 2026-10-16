package com.tonypine.jobsearchhub.core

/** How loudly a channel interrupts until the user changes it: a heads-up, a sound, or quietly in the shade. */
enum class NoticeImportance { HIGH, DEFAULT, LOW }

/**
 * The notification channel an update posts on. Each kind has its own, so the
 * user can mute matches in the system's settings and keep follow-ups.
 */
enum class NoticeChannel(val id: String, val title: String, val description: String, val importance: NoticeImportance) {
    FOLLOW_UPS("follow_ups", "Follow-ups", "A card on the pipeline is due a follow-up.", NoticeImportance.HIGH),
    REPLIES("replies", "Replies", "A company or a recruiter wrote: a reply, an interview invite, or an application's end.", NoticeImportance.HIGH),
    MATCHES("matches", "Matches", "A fresh job is a strong match.", NoticeImportance.DEFAULT),
    HUB("hub", "Hub", "The Mac finished a request, something the phone asked of the hub failed, or the app needs updating.", NoticeImportance.DEFAULT),
    ;

    /** Two or more of the channel's notifications, as the summary that groups them says it. */
    fun summary(count: Int): String = when (this) {
        FOLLOW_UPS -> if (count == 1) "1 follow-up due" else "$count follow-ups due"
        REPLIES -> if (count == 1) "1 reply" else "$count replies"
        MATCHES -> if (count == 1) "1 fresh strong match" else "$count fresh strong matches"
        HUB -> if (count == 1) "1 hub update" else "$count hub updates"
    }

    companion object {
        /** The id of the one channel the app had before there was one per kind. */
        const val LEGACY_ID = "updates"

        /** The fewest notifications of a kind that group under a summary. */
        const val GROUP_FROM = 2

        /** The kinds of mail the hub tells of: someone at a company wrote. */
        private val replyKinds = setOf("reply", "human_reply", "interview_invite", "recruiter_outreach", "rejection", "application_confirmation")

        /** The channel for an update of the kind; one the phone doesn't know goes on Hub. */
        fun of(kind: String?): NoticeChannel = when (kind) {
            HubUpdate.FOLLOW_UP_DUE -> FOLLOW_UPS
            in replyKinds -> REPLIES
            HubUpdate.FRESH_MATCH -> MATCHES
            else -> HUB
        }
    }
}

/** A button on a notification. */
enum class NoticeAction(val id: String, val title: String) {
    /** Records the follow-up through the hub, which sets the next date as the phase asks. */
    FOLLOWED_UP("followed_up", "Followed up"),
    SNOOZE_A_DAY("snooze_a_day", "Snooze a day"),
    OPEN("open", "Open"),
    MARK_AS_READ("mark_as_read", "Mark as read"),
    ;

    /** What the notification on Hub says when the action didn't reach the hub. */
    fun failure(update: PushedUpdate, reason: String): Notice = when (this) {
        FOLLOWED_UP -> Notice("Couldn't record the follow-up", "${update.title}: $reason")
        MARK_AS_READ -> Notice("Couldn't mark the reply as read", "${update.title}: $reason")
        SNOOZE_A_DAY, OPEN -> Notice("$title failed", "${update.title}: $reason")
    }

    companion object {
        fun of(id: String?): NoticeAction? = entries.firstOrNull { it.id == id }

        /** Why Followed up found nothing to record. */
        const val NO_CARD = "The pipeline has no card for it anymore."

        /** Why a button found no hub to tell: the phone was unpaired since the notification came. */
        const val NOT_PAIRED = "This phone isn't paired with a hub anymore."
    }
}

/** A notification's title and text, as the phone words it. */
data class Notice(val title: String, val text: String)
