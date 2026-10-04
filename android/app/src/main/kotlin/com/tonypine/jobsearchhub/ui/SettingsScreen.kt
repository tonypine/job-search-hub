package com.tonypine.jobsearchhub.ui

import android.content.Intent
import android.provider.Settings
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Laptop
import androidx.compose.material.icons.rounded.LinkOff
import androidx.compose.material.icons.rounded.Notifications
import androidx.compose.material.icons.rounded.Sync
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.app.NotificationManagerCompat
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.push.UpdateNotifications
import com.tonypine.jobsearchhub.ui.design.HubSection
import com.tonypine.jobsearchhub.ui.design.SegmentedGroup
import com.tonypine.jobsearchhub.ui.design.Spacing
import java.net.URI

/** The paired hub, the phone's notifications, and unpairing, which asks first. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(state: HubState, onBack: () -> Unit, onUnpair: () -> Unit) {
    val context = LocalContext.current
    var isUnpairing by rememberSaveable { mutableStateOf(false) }
    // Read again on each return, as the switch lives in the system's settings.
    var notificationsOn by remember { mutableStateOf(true) }
    LifecycleResumeEffect(Unit) {
        notificationsOn = NotificationManagerCompat.from(context).areNotificationsEnabled()
        onPauseOrDispose {}
    }
    val pushesAvailable = remember { UpdateNotifications.isAvailable(context) }

    if (isUnpairing) {
        AlertDialog(
            onDismissRequest = { isUnpairing = false },
            icon = { Icon(Icons.Rounded.LinkOff, contentDescription = null, tint = MaterialTheme.colorScheme.error) },
            title = { Text("Unpair this phone?") },
            text = { Text("It stops getting updates and reminders until you pair it again from the Mac.") },
            confirmButton = {
                TextButton(onClick = { isUnpairing = false; onUnpair() }) { Text("Unpair", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { isUnpairing = false }) { Text("Cancel") } },
        )
    }

    Column {
        TopAppBar(
            title = { Text("Settings") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back") } },
        )
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(Spacing.l)) {
            val hubUrl = state.pairing?.hubUrl.orEmpty()
            HubSection("Hub") {
                SegmentedGroup(
                    listOf(
                        { SettingsRow(Icons.Rounded.Laptop, "Paired with", describeHub(hubUrl)) },
                        {
                            SettingsRow(
                                Icons.Rounded.Sync, "Last read",
                                when {
                                    state.error != null -> "Can't reach the hub right now"
                                    state.readAt != null -> formatWhen(state.readAt.toString())
                                    else -> "Not yet"
                                },
                            )
                        },
                    ),
                    Modifier.padding(horizontal = Spacing.l),
                )
            }
            HubSection("Notifications") {
                SegmentedGroup(
                    listOf {
                        SettingsRow(
                            Icons.Rounded.Notifications, "Updates and reminders",
                            when {
                                !pushesAvailable -> "Off in this build, which has no Firebase config"
                                notificationsOn -> "On. Change them in the system's settings"
                                else -> "Off. Turn them on in the system's settings"
                            },
                            onClick = if (pushesAvailable) {
                                {
                                    context.startActivity(
                                        Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName),
                                    )
                                }
                            } else {
                                null
                            },
                        )
                    },
                    Modifier.padding(horizontal = Spacing.l),
                )
            }
            SegmentedGroup(
                listOf { SettingsRow(Icons.Rounded.LinkOff, "Unpair this phone", color = MaterialTheme.colorScheme.error) { isUnpairing = true } },
                Modifier.padding(horizontal = Spacing.l),
            )
        }
    }
}

/** The hub's host, and how the phone reaches it. */
private fun describeHub(hubUrl: String): String {
    val host = runCatching { URI(hubUrl).host }.getOrNull() ?: return hubUrl
    return when {
        host.endsWith(".ts.net") -> "$host, over Tailscale"
        host == "10.0.2.2" -> "This Mac, from the emulator"
        else -> host
    }
}

@Composable
private fun SettingsRow(
    icon: ImageVector,
    headline: String,
    supporting: String? = null,
    color: Color? = null,
    onClick: (() -> Unit)? = null,
) {
    Row(
        Modifier.fillMaxWidth().then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier).heightIn(min = 64.dp)
            .padding(horizontal = Spacing.l, vertical = Spacing.m),
        horizontalArrangement = Arrangement.spacedBy(Spacing.l),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = color ?: MaterialTheme.colorScheme.onSurfaceVariant)
        Column(Modifier.weight(1f)) {
            Text(headline, style = MaterialTheme.typography.bodyLarge, color = color ?: MaterialTheme.colorScheme.onSurface)
            supporting?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
    }
}
