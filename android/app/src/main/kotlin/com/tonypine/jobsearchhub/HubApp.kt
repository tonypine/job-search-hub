package com.tonypine.jobsearchhub

import android.app.Application
import com.tonypine.jobsearchhub.core.PushedUpdate
import com.tonypine.jobsearchhub.data.PairingStore
import com.tonypine.jobsearchhub.push.UpdateNotifications
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow

class HubApp : Application() {
    val pairingStore by lazy { PairingStore(this) }

    /** Work that outlives a screen, like sending a rotated push token. */
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /** Each update pushed while the app runs, so its lists read the hub again. */
    val pushes = MutableSharedFlow<PushedUpdate>(extraBufferCapacity = 1, onBufferOverflow = BufferOverflow.DROP_OLDEST)

    override fun onCreate() {
        super.onCreate()
        UpdateNotifications.createChannels(this)
    }
}
