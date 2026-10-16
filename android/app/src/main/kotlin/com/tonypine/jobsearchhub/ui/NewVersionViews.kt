package com.tonypine.jobsearchhub.ui

import android.content.Intent
import android.provider.Settings
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Download
import androidx.compose.material.icons.rounded.TaskAlt
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.tonypine.jobsearchhub.core.NewVersions
import com.tonypine.jobsearchhub.versions.InstallStep
import com.tonypine.jobsearchhub.versions.VersionState
import com.tonypine.jobsearchhub.versions.VersionUpdater
import com.tonypine.jobsearchhub.versions.open
import com.tonypine.jobsearchhub.ui.design.HubCard
import com.tonypine.jobsearchhub.ui.design.Spacing

/** What the card, Settings and the too-old screen do with the owner's taps on a new version. */
class VersionActions(
    val onUpdate: () -> Unit,
    val onLater: () -> Unit,
    val onCheckNow: () -> Unit,
    val onAllowInstalls: () -> Unit,
    val onDismissInstall: () -> Unit,
    val onDismissNowOn: () -> Unit,
)

/** The actions on the app's one [VersionUpdater], with *Allow* sending the owner to the system's *Install unknown apps*. */
@Composable
fun rememberVersionActions(updater: VersionUpdater): VersionActions {
    val context = LocalContext.current
    return VersionActions(
        onUpdate = updater::update,
        onLater = updater::later,
        onCheckNow = { updater.checkNow() },
        onAllowInstalls = {
            updater.askedForInstalls()
            context.startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, "package:${context.packageName}".toUri()))
        },
        onDismissInstall = updater::dismissInstall,
        onDismissNowOn = updater::dismissNowOn,
    )
}

/**
 * Opens Android's confirmation once the installer asks for it and the app is in front, and carries on an Update that
 * waited for the owner to allow installs, when they come back from the system's settings.
 */
@Composable
fun InstallPrompts(updater: VersionUpdater, state: VersionState) {
    val context = LocalContext.current
    LifecycleResumeEffect(Unit) {
        updater.resumeIfAllowed()
        onPauseOrDispose {}
    }
    val step = state.install
    LaunchedEffect(step) {
        if (step is InstallStep.Confirm) {
            updater.confirmationShown()
            if (!step.open(context::startActivity)) updater.onInstallStatus(InstallStep.Failed("Android's installer didn't open."))
        }
    }
}

/**
 * Today's card for a new version: "Version 0.1.253 is ready" with two of its changes, Update and Later; then the
 * update's progress, the reason it needs the owner's permission, or what failed; and "Now on" once it's done.
 */
@Composable
fun NewVersionCard(state: VersionState, actions: VersionActions, modifier: Modifier = Modifier) {
    val ready = state.ready
    val nowOn = state.nowOn
    Column(
        modifier.fillMaxWidth().padding(horizontal = Spacing.l).clip(RoundedCornerShape(20.dp))
            .background(MaterialTheme.colorScheme.primaryContainer).padding(Spacing.l),
        verticalArrangement = Arrangement.spacedBy(Spacing.s),
    ) {
        when {
            nowOn != null && state.install == InstallStep.Idle -> {
                CardTitle(Icons.Rounded.TaskAlt, "Now on $nowOn")
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    TextButton(onClick = actions.onDismissNowOn) { Text("OK") }
                }
            }
            else -> {
                val version = ready?.version
                val title = when (state.install) {
                    InstallStep.Idle, InstallStep.NeedsPermission, is InstallStep.Failed -> version?.let { "Version $it is ready" } ?: "A new version"
                    else -> version?.let { "Updating to $it…" } ?: "Updating…"
                }
                CardTitle(Icons.Rounded.Download, title)
                ready?.changelog?.highlights()?.takeIf { it.isNotEmpty() }?.let { Bullets(it) }
                if (state.install == InstallStep.Idle) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(Spacing.s, Alignment.End)) {
                        TextButton(onClick = actions.onLater) { Text("Later") }
                        Button(onClick = actions.onUpdate) { Text("Update") }
                    }
                } else {
                    InstallProgress(state.install, actions)
                }
            }
        }
    }
}

/** Settings › App version: the installed version and the newest, what's new in it, Check now and Update. */
@Composable
fun AppVersionCard(state: VersionState, actions: VersionActions, modifier: Modifier = Modifier) {
    val ready = state.ready
    HubCard(modifier.padding(horizontal = Spacing.l)) {
        Text(
            state.installed + when {
                ready != null -> " · ${ready.version} is ready"
                state.isChecking -> " · Checking…"
                state.checkedAt != null && state.problem == null -> " · Up to date"
                else -> ""
            },
            style = MaterialTheme.typography.titleMedium,
        )
        state.checkedAt?.let { Caption("Checked ${formatWhen(it.toString())}") }
        state.problem?.let { Caption(it) }
        ready?.changelog?.let { changelog ->
            if (changelog.new.isNotEmpty()) {
                Caption("New")
                Bullets(changelog.new)
            }
            if (changelog.fixed.isNotEmpty()) {
                Caption("Fixed")
                Bullets(changelog.fixed)
            }
        }
        if (state.install == InstallStep.Idle) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = actions.onCheckNow, enabled = !state.isChecking) { Text("Check now") }
                Box(Modifier.weight(1f))
                if (ready != null) Button(onClick = actions.onUpdate) { Text("Update") }
            }
        } else {
            InstallProgress(state.install, actions)
        }
    }
}

/** In place of the pages and their errors, when the hub no longer serves this version of the app: its words, and Update. */
@Composable
fun TooOldScreen(message: String, state: VersionState, actions: VersionActions, onRetry: () -> Unit) {
    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(Spacing.xl),
        verticalArrangement = Arrangement.spacedBy(Spacing.l, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Box(
            Modifier.size(64.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.primary),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Rounded.Download, contentDescription = null, tint = MaterialTheme.colorScheme.onPrimary, modifier = Modifier.size(36.dp))
        }
        Text(
            NewVersions.TOO_OLD, style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center,
            modifier = Modifier.semantics { heading() },
        )
        Text(message, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
        if (state.install == InstallStep.Idle) {
            Button(onClick = actions.onUpdate) { Text(state.ready?.let { "Update to ${it.version}" } ?: "Update") }
            TextButton(onClick = onRetry) { Text("Try again") }
        } else {
            Column(Modifier.widthIn(max = 420.dp)) { InstallProgress(state.install, actions) }
        }
    }
}

/** An update under way: its progress, Android's confirmation, the reason to allow installs from the hub, or what failed. */
@Composable
private fun InstallProgress(step: InstallStep, actions: VersionActions) {
    when (step) {
        InstallStep.Idle -> Unit
        is InstallStep.Working -> {
            val progress = step.progress
            if (progress == null) LinearProgressIndicator(Modifier.fillMaxWidth()) else LinearProgressIndicator({ progress }, Modifier.fillMaxWidth())
        }
        is InstallStep.Confirm, InstallStep.Confirming -> Caption("Confirm the update in Android's dialog. The app restarts on the new version.")
        InstallStep.NeedsPermission -> {
            Text(
                "To update itself, the app needs your permission to install apps. Android asks once: turn on " +
                    "Allow from this source for Job Search Hub, then come back.",
                style = MaterialTheme.typography.bodyMedium,
            )
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(Spacing.s, Alignment.End)) {
                TextButton(onClick = actions.onDismissInstall) { Text("Cancel") }
                Button(onClick = actions.onAllowInstalls) { Text("Continue") }
            }
        }
        is InstallStep.Failed -> {
            Text(step.message, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(Spacing.s, Alignment.End)) {
                TextButton(onClick = actions.onDismissInstall) { Text("Close") }
                Button(onClick = { actions.onDismissInstall(); actions.onUpdate() }) { Text("Try again") }
            }
        }
    }
}

@Composable
private fun CardTitle(icon: ImageVector, title: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
        Text(title, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onPrimaryContainer)
    }
}

@Composable
private fun Bullets(lines: List<String>) {
    Column(verticalArrangement = Arrangement.spacedBy(Spacing.xs)) {
        lines.forEach { Text("• $it", style = MaterialTheme.typography.bodyMedium) }
    }
}

@Composable
private fun Caption(text: String) {
    Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
}
