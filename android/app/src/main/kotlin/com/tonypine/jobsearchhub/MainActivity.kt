package com.tonypine.jobsearchhub

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import com.tonypine.jobsearchhub.ui.HubTheme
import com.tonypine.jobsearchhub.ui.HubNavigation

class MainActivity : ComponentActivity() {
    private val viewModel: HubViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pairFrom(intent)
        setContent {
            HubTheme {
                Surface(color = MaterialTheme.colorScheme.background) {
                    HubNavigation(viewModel)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        pairFrom(intent)
    }

    /** A pairing link opened on the phone pairs it, like the QR code. */
    private fun pairFrom(intent: Intent?) {
        val link = intent?.data?.toString() ?: return
        if (link.startsWith("jobsearchhub://pair")) {
            viewModel.pair(link)
        }
    }
}
