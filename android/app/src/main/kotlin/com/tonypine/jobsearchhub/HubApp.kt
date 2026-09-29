package com.tonypine.jobsearchhub

import android.app.Application
import com.tonypine.jobsearchhub.data.PairingStore

class HubApp : Application() {
    val pairingStore by lazy { PairingStore(this) }
}
