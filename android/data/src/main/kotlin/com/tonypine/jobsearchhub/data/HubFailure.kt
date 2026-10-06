package com.tonypine.jobsearchhub.data

/**
 * A failed call as an error view shows it. A hub that no longer serves this version of the app gets its own headline,
 * and its message says what to do, where any other failure gets the screen's headline, network advice and the message
 * behind Details.
 */
data class HubFailure(val message: String, val isUpgradeRequired: Boolean = false) {
    /** The headline: [UPGRADE_TITLE] for an app the hub no longer serves, or else [otherwise], the screen's own. */
    fun title(otherwise: String): String = if (isUpgradeRequired) UPGRADE_TITLE else otherwise

    /** What to do: the hub's words for an app it no longer serves, or null for the view's network advice. */
    val advice: String? get() = message.takeIf { isUpgradeRequired }

    /** The raw error behind Details; none for an app the hub no longer serves, whose message is already the advice. */
    val details: String? get() = message.takeUnless { isUpgradeRequired }

    companion object {
        const val UPGRADE_TITLE = "Update the app"

        /** The failure as the phone shows it, from whatever a call threw. */
        fun of(error: Throwable): HubFailure =
            HubFailure(error.message ?: error.toString(), isUpgradeRequired = (error as? HubException)?.isUpgradeRequired == true)
    }
}
