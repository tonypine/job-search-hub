package com.tonypine.jobsearchhub.data

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class HubFailureTest {
    @Test
    fun anAppTheHubNoLongerServesShowsTheHubsWordsAsTheAdvice() {
        val message = "This app (0.1.268) is too old for the hub, which runs 0.1.310. Update the app to 0.1.300 or later."
        val failure = HubFailure.of(HubException(message, isUpgradeRequired = true))
        assertEquals("Update the app", failure.title("Couldn't reach the hub"))
        assertEquals(message, failure.advice)
        assertNull(failure.details)
    }

    @Test
    fun anyOtherFailureKeepsTheScreensHeadlineAndShowsItsMessageBehindDetails() {
        val failure = HubFailure.of(HubException("Can't reach the hub at http://mac.local:8090: timeout"))
        assertEquals("Couldn't reach the hub", failure.title("Couldn't reach the hub"))
        assertNull(failure.advice)
        assertEquals("Can't reach the hub at http://mac.local:8090: timeout", failure.details)
        assertEquals(HubFailure("boom"), HubFailure.of(IllegalStateException("boom")))
    }
}
