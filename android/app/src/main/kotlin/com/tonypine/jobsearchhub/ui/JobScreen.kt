package com.tonypine.jobsearchhub.ui

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Help
import androidx.compose.material.icons.filled.Info
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.core.JobBrief
import com.tonypine.jobsearchhub.core.JobBriefPoint
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobScreen(id: String, viewModel: HubViewModel, onBack: () -> Unit, onOpenCompany: (String) -> Unit, onDecided: (String?) -> Unit) {
    var details by remember { mutableStateOf<JobDetails?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var isAskingForSkip by remember { mutableStateOf(false) }
    var reason by remember { mutableStateOf("") }
    var isAskingForFix by remember { mutableStateOf(false) }
    var note by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(id) {
        viewModel.loadJob(id).onSuccess { details = it }.onFailure { error = it.message }
    }
    val decide: (String, String) -> Unit = { decision, why ->
        scope.launch { viewModel.decideJob(id, decision, why).fold({ next -> onDecided(next) }, { error = it.message }) }
    }
    if (isAskingForSkip) {
        AlertDialog(
            onDismissRequest = { isAskingForSkip = false },
            title = { Text("Skip this job?") },
            text = {
                Column {
                    Text("It leaves the jobs list and stays dismissed when its board lists it again. Restore it from the Mac.")
                    OutlinedTextField(reason, { reason = it }, label = { Text("Reason (optional)") })
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    isAskingForSkip = false
                    decide("skip", reason)
                }) { Text("Skip") }
            },
            dismissButton = { TextButton(onClick = { isAskingForSkip = false }) { Text("Cancel") } },
        )
    }
    if (isAskingForFix) {
        AlertDialog(
            onDismissRequest = { isAskingForFix = false },
            title = { Text("Fix this job?") },
            text = {
                Column {
                    Text("Say what's wrong. An agent on the Mac corrects the details, and the outcome comes as an update.")
                    OutlinedTextField(note, { note = it }, label = { Text("What's wrong") })
                }
            },
            confirmButton = {
                TextButton(enabled = note.isNotBlank(), onClick = {
                    isAskingForFix = false
                    scope.launch {
                        message = viewModel.askTheMac(QueueTaskRequest(kind = "fix_job", jobId = id, note = note.trim()))
                            .fold({ "Sent to the Mac. The result comes as an update." }, { it.message })
                    }
                }) { Text("Fix") }
            },
            dismissButton = { TextButton(onClick = { isAskingForFix = false }) { Text("Cancel") } },
        )
    }
    Column(Modifier.fillMaxSize()) {
        TopAppBar(
            title = { Text(details?.companyName ?: "Job") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back") } },
        )
        when {
            details != null -> JobDetailsView(
                details!!, onOpenCompany, onSkip = { isAskingForSkip = true }, onDecide = { decide(it, "") },
                onFix = { isAskingForFix = true }, message = message, error = error,
            )
            error != null -> Text(error!!, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp))
            else -> CircularProgressIndicator(Modifier.padding(24.dp).align(Alignment.CenterHorizontally))
        }
    }
}

@Composable
private fun JobDetailsView(
    details: JobDetails,
    onOpenCompany: (String) -> Unit,
    onSkip: () -> Unit,
    onDecide: (String) -> Unit,
    onFix: () -> Unit,
    message: String?,
    error: String?,
) {
    val context = LocalContext.current
    Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(details.job.title, style = MaterialTheme.typography.titleLarge)
        Text(listOfNotNull(details.companyName, details.job.location, details.job.workplaceType).joinToString(" · "), color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { context.startActivity(Intent(Intent.ACTION_VIEW, details.job.url.toUri())) }) { Text("Open posting") }
            details.job.companyId?.let { companyId ->
                OutlinedButton(onClick = { onOpenCompany(companyId) }) { Text("Company brief") }
            }
            OutlinedButton(onClick = onFix) { Text("Fix…") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { onDecide("pursue") }) { Text("Pursue") }
            OutlinedButton(onClick = onSkip) { Text("Skip") }
            OutlinedButton(onClick = { onDecide("later") }) { Text("Later") }
        }
        details.decision?.let { Text(describeDecision(it.decision, it.reason), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        message?.let { Text(it, color = MaterialTheme.colorScheme.primary) }
        error?.let { Text(it, color = MaterialTheme.colorScheme.error) }

        details.brief?.let { brief ->
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Brief", fontWeight = FontWeight.SemiBold)
                MatchLabel(brief.match)
                Text(if (brief.isFull) "by Claude" else "by the local model", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text(brief.reason)
            BriefPoints("Strengths", brief.strengths, brief, Icons.Filled.CheckCircle, Color(0xFF2E9E4F))
            BriefPoints("Weaknesses", brief.weaknesses, brief, Icons.Filled.Cancel, Color(0xFFD08A00))
        }
        if (details.screenOut.isNotEmpty()) {
            Text("Screen-out checks", fontWeight = FontWeight.SemiBold)
            details.screenOut.forEach { answer ->
                Row(verticalAlignment = Alignment.Top) {
                    val (icon, tint) = when (answer.verdict) {
                        "yes" -> Icons.Filled.CheckCircle to Color(0xFF2E9E4F)
                        "no" -> Icons.Filled.Cancel to MaterialTheme.colorScheme.error
                        null -> Icons.Filled.Info to MaterialTheme.colorScheme.onSurfaceVariant
                        else -> Icons.Filled.Help to Color(0xFFD08A00)
                    }
                    Icon(icon, contentDescription = answer.verdict, tint = tint)
                    Column(Modifier.padding(start = 8.dp)) {
                        Text("${answer.name}: ${answer.answer}")
                        answer.evidence?.let { Text("\u201C$it\u201D", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                    }
                }
            }
        }

        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("Fit  ", fontWeight = FontWeight.SemiBold)
            FitLabel(details.fit.level)
        }
        details.fit.checks.forEach { check ->
            Row(verticalAlignment = Alignment.Top) {
                val (icon, tint) = when (check.verdict) {
                    "yes" -> Icons.Filled.CheckCircle to Color(0xFF2E9E4F)
                    "no" -> Icons.Filled.Cancel to MaterialTheme.colorScheme.error
                    else -> Icons.Filled.Help to Color(0xFFD08A00)
                }
                Icon(icon, contentDescription = check.verdict, tint = tint)
                Text("  ${check.name}: ${check.reason}")
            }
        }

        details.connections?.takeIf { it.isNotEmpty() }?.let { connections ->
            Text("People you know there", fontWeight = FontWeight.SemiBold)
            connections.forEach { person ->
                Text("${person.firstName} ${person.lastName}" + (person.position?.let { " · $it" } ?: ""))
            }
        }

        details.facts?.entries?.let { entries ->
            val shown = entries.mapNotNull { entry -> describe(entry.value)?.let { entry.title to it } }
            if (shown.isNotEmpty()) {
                Text("Read from the posting", fontWeight = FontWeight.SemiBold)
                shown.forEach { (title, value) -> Text("$title: $value", style = MaterialTheme.typography.bodyMedium) }
            }
        }

        details.job.description?.takeIf { it.isNotBlank() }?.let {
            Text("The posting", fontWeight = FontWeight.SemiBold)
            Text(it, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** A fact as text; nil when the posting doesn't say. */
private fun describe(value: kotlinx.serialization.json.JsonElement): String? = when (value) {
    is JsonNull -> null
    is JsonPrimitive -> value.contentOrNull?.takeIf { it.isNotBlank() && !it.equals("not stated", ignoreCase = true) }
    is JsonArray -> value.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.filter { it.isNotBlank() }.joinToString(", ").takeIf { it.isNotEmpty() }
    else -> null
}

@Composable
private fun BriefPoints(title: String, points: List<JobBriefPoint>, brief: JobBrief, icon: ImageVector, tint: Color) {
    if (points.isEmpty()) return
    Text(title, style = MaterialTheme.typography.titleSmall)
    points.forEach { point ->
        Row(verticalAlignment = Alignment.Top) {
            Icon(icon, contentDescription = null, tint = tint)
            Column(Modifier.padding(start = 8.dp)) {
                Text(point.point)
                val entries = brief.entriesOf(point)
                if (entries.isNotEmpty()) {
                    Text(entries.joinToString("; ") { it.label }, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

/** The decision as the screen reads it: "Left for later", "Skipped: agency". */
private fun describeDecision(decision: String, reason: String?): String {
    val made = when (decision) {
        "pursue" -> "Pursued"
        "skip" -> "Skipped"
        else -> "Left for later"
    }
    return if (reason.isNullOrBlank()) made else "$made: $reason"
}
