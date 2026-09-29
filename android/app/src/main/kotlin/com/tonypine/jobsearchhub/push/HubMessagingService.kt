package com.tonypine.jobsearchhub.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.tonypine.jobsearchhub.HubApp
import com.tonypine.jobsearchhub.core.PushedUpdate
import com.tonypine.jobsearchhub.data.HubClient
import com.tonypine.jobsearchhub.data.HubException
import kotlinx.coroutines.launch

/** Receives the hub's pushes, and tells the hub when FCM rotates this app's token. */
class HubMessagingService : FirebaseMessagingService() {
    private val app get() = application as HubApp

    override fun onMessageReceived(message: RemoteMessage) {
        val update = PushedUpdate.parse(message.data) ?: return
        UpdateNotifications.show(this, update)
        app.pushes.tryEmit(update)
    }

    override fun onNewToken(token: String) {
        val pairing = app.pairingStore.load() ?: return
        app.scope.launch {
            try {
                HubClient(pairing).setPushToken(token)
            } catch (_: HubException) {
                // The app registers its token again each time it starts.
            }
        }
    }
}
