package com.tonypine.jobsearchhub.ui

import android.content.Context
import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.OpenInNew
import androidx.compose.material.icons.rounded.Build
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.RemoveCircle
import androidx.compose.material.icons.rounded.Share
import androidx.compose.material.icons.rounded.ThumbUp
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.core.JobBrief
import com.tonypine.jobsearchhub.core.JobBriefPoint
import com.tonypine.jobsearchhub.core.JobDecision
import com.tonypine.jobsearchhub.core.JobDetails
import com.tonypine.jobsearchhub.core.Match
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.core.Screen
import com.tonypine.jobsearchhub.core.SetAside
import com.tonypine.jobsearchhub.core.Tone
import com.tonypine.jobsearchhub.core.screenRows
import com.tonypine.jobsearchhub.ui.design.ActionBar
import com.tonypine.jobsearchhub.ui.design.EntityHeader
import com.tonypine.jobsearchhub.ui.design.FactGrid
import com.tonypine.jobsearchhub.ui.design.HubAction
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.HubSection
import com.tonypine.jobsearchhub.ui.design.HubCard
import com.tonypine.jobsearchhub.ui.design.OverflowMenu
import com.tonypine.jobsearchhub.ui.design.ParentLink
import com.tonypine.jobsearchhub.ui.design.PersonRow
import com.tonypine.jobsearchhub.ui.design.Relation
import com.tonypine.jobsearchhub.ui.design.SegmentedGroup
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip
import com.tonypine.jobsearchhub.ui.design.VerdictRow
import com.tonypine.jobsearchhub.ui.design.symbol
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/**
 * A job: its header, then Overview (Brief, Screen), People and Posting as tabs,
 * the same split as the Mac's inspector. The decision is docked at the bottom,
 * with Pursue the primary. Open posting is in the app bar, Fix… and Share in
 * the overflow. Beside the list it has no back arrow ([onBack] is null), and
 * back returns to the list all the same.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobScreen(id: String, viewModel: HubViewModel, onBack: (() -> Unit)?, onOpenCompany: (String) -> Unit, onDecided: (String?) -> Unit) {
    val context = LocalContext.current
    var details by remember { mutableStateOf<JobDetails?>(null) }
    var loadError by remember { mutableStateOf<String?>(null) }
    var attempt by remember { mutableIntStateOf(0) }
    var error by remember { mutableStateOf<String?>(null) }
    var isDeciding by remember { mutableStateOf(false) }
    var isAskingForSkip by remember { mutableStateOf(false) }
    var reason by remember { mutableStateOf("") }
    var isAskingForFix by remember { mutableStateOf(false) }
    var note by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(id, attempt) {
        loadError = null
        viewModel.loadJob(id).onSuccess { details = it }.onFailure { loadError = it.message ?: it.toString() }
    }
    val decide: (String, String) -> Unit = { decision, why ->
        isDeciding = true
        error = null
        scope.launch {
            viewModel.decideJob(id, decision, why).fold({ next -> onDecided(next) }, { error = it.message ?: it.toString() })
            isDeciding = false
        }
    }
    if (isAskingForSkip) {
        AlertDialog(
            onDismissRequest = { isAskingForSkip = false },
            title = { Text("Skip this job?") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(Spacing.m)) {
                    Text("It leaves the jobs list and stays skipped when its board lists it again. Restore it from the Mac.")
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
                Column(verticalArrangement = Arrangement.spacedBy(Spacing.m)) {
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
            title = {},
            navigationIcon = { onBack?.let { IconButton(onClick = it) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back") } } },
            actions = {
                details?.let { shown ->
                    IconButton(onClick = { openPosting(context, shown) }) {
                        Icon(Icons.AutoMirrored.Rounded.OpenInNew, contentDescription = "Open posting")
                    }
                    OverflowMenu(
                        listOf(
                            HubAction("Fix…", Icons.Rounded.Build) { isAskingForFix = true },
                            HubAction("Share", Icons.Rounded.Share) { sharePosting(context, shown) },
                        ),
                    )
                }
            },
        )
        val shown = details
        when {
            shown != null -> {
                JobDetailsView(shown, onOpenCompany, message = message, modifier = Modifier.weight(1f))
                // Above the docked bar, so a failed decision shows wherever the details are scrolled.
                error?.let { HubErrorView("Couldn't record your decision", it) }
                ActionBar(
                    primary = HubAction("Pursue", Icons.Rounded.Check, enabled = !isDeciding) { decide("pursue", "") },
                    tonal = HubAction("Later", enabled = !isDeciding) { decide("later", "") },
                    outlined = HubAction("Skip…", enabled = !isDeciding) { isAskingForSkip = true },
                )
            }
            loadError != null -> HubErrorView("Couldn't load this job", loadError, onRetry = { attempt++ })
            else -> CircularProgressIndicator(Modifier.padding(Spacing.xl).align(Alignment.CenterHorizontally))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun JobDetailsView(details: JobDetails, onOpenCompany: (String) -> Unit, message: String?, modifier: Modifier = Modifier) {
    var tab by rememberSaveable { mutableIntStateOf(0) }
    val people = details.connections.orEmpty()
    // A board job not yet linked to a company still names it, as plain text in the facts.
    val company = details.companyName?.takeIf { it.isNotBlank() }
    val companyId = details.job.companyId
    Column(modifier.verticalScroll(rememberScrollState()).padding(bottom = Spacing.l)) {
        EntityHeader(
            title = details.job.title,
            parent = company?.let { name -> companyId?.let { ParentLink(name) { onOpenCompany(it) } } },
            facts = listOfNotNull(
                company.takeIf { companyId == null }, details.job.location, details.job.workplaceType, formatWhen(details.job.firstSeenAt),
            ).joinToString(" · "),
            chips = {
                details.brief?.let { brief ->
                    val match = Match.of(brief.match)
                    ToneChip(match.label, match.tone, icon = if (match == Match.STRONG) Icons.Rounded.ThumbUp else null)
                }
                val screen = Screen.ofLevel(details.fit.level)
                ToneChip(screen.label, screen.tone, icon = screen.symbol)
                details.decision?.let { DecisionChip(it) }
            },
        )
        Column(Modifier.padding(horizontal = Spacing.l, vertical = Spacing.s), verticalArrangement = Arrangement.spacedBy(Spacing.xs)) {
            details.decision?.reason?.takeIf { it.isNotBlank() }?.let {
                Text("Why: $it", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            message?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.primary) }
        }
        PrimaryTabRow(selectedTabIndex = tab, containerColor = MaterialTheme.colorScheme.surface) {
            Tab(selected = tab == 0, onClick = { tab = 0 }, text = { Text("Overview") })
            Tab(selected = tab == 1, onClick = { tab = 1 }, text = { Text(if (people.isEmpty()) "People" else "People · ${people.size}") })
            Tab(selected = tab == 2, onClick = { tab = 2 }, text = { Text("Posting") })
        }
        Column(Modifier.padding(horizontal = Spacing.m).padding(top = Spacing.l), verticalArrangement = Arrangement.spacedBy(Spacing.l)) {
            when (tab) {
                0 -> {
                    details.brief?.let { BriefSection(it) }
                    ScreenSection(details)
                }
                1 -> PeopleSection(details)
                else -> PostingSections(details)
            }
        }
    }
}

/** The decision already made, as a chip: Pursued, Later or Skipped. */
@Composable
private fun DecisionChip(decision: JobDecision) {
    when (decision.decision) {
        "pursue" -> ToneChip("Pursued", Tone.ACCENT)
        "skip" -> ToneChip(SetAside.SKIPPED.word, SetAside.SKIPPED.tone)
        else -> ToneChip("Later", Tone.NEUTRAL)
    }
}

@Composable
private fun BriefSection(brief: JobBrief) {
    HubSection("Brief", trailing = {
        Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
            if (brief.isStale) ToneChip("Stale", Tone.CAUTION)
            Text(
                if (brief.isFull) "by Claude" else "by the local model",
                style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }) {
        val reason: @Composable () -> Unit = {
            Text(brief.reason, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(Spacing.l))
        }
        val strengths = brief.strengths.map { point -> briefPoint(brief, point, Tone.POSITIVE) }
        val weaknesses = brief.weaknesses.map { point -> briefPoint(brief, point, Tone.CAUTION) }
        SegmentedGroup(listOf(reason) + strengths + weaknesses)
    }
}

private fun briefPoint(brief: JobBrief, point: JobBriefPoint, tone: Tone): @Composable () -> Unit = {
    val isStrength = tone == Tone.POSITIVE
    VerdictRow(
        name = point.point, tone = tone,
        icon = if (isStrength) Icons.Rounded.CheckCircle else Icons.Rounded.RemoveCircle,
        verdict = if (isStrength) "Strength" else "Weakness",
        reason = brief.entriesOf(point).joinToString("; ") { it.label },
    )
}

/** The fit checks and the screen-out answers together: does anything rule the owner out? */
@Composable
private fun ScreenSection(details: JobDetails) {
    val rows = details.screenRows()
    if (rows.isEmpty()) return
    HubSection("Screen") {
        SegmentedGroup(
            rows.map { row ->
                {
                    VerdictRow(
                        name = row.name, tone = row.tone, icon = row.screen.symbol, verdict = row.screen?.word ?: "Information",
                        reason = row.reason, evidence = row.evidence,
                    )
                }
            },
        )
    }
}

@Composable
private fun PeopleSection(details: JobDetails) {
    val people = details.connections.orEmpty()
    HubSection("People") {
        if (people.isEmpty()) {
            Text(
                "None of your connections work at ${details.companyName ?: "this company"}.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = Spacing.l),
            )
        } else {
            SegmentedGroup(
                people.map { person ->
                    { PersonRow("${person.firstName} ${person.lastName}".trim(), Relation.CONNECTION, role = person.position) }
                },
            )
        }
    }
}

@Composable
private fun PostingSections(details: JobDetails) {
    val facts = details.facts?.entries.orEmpty().mapNotNull { entry -> describe(entry.value)?.let { entry.title to it } }
    val board = listOfNotNull(
        "Board" to details.job.source,
        details.job.employmentType?.let { "Employment" to it },
        "First seen" to formatWhen(details.job.firstSeenAt),
    )
    HubSection("From the board") { HubCard { FactGrid(board) } }
    if (facts.isNotEmpty()) {
        HubSection("Read from the posting") { HubCard { FactGrid(facts) } }
    }
    details.job.description?.takeIf { it.isNotBlank() }?.let {
        HubSection("Posting") { HubCard { Text(it, style = MaterialTheme.typography.bodyMedium) } }
    }
}

private fun openPosting(context: Context, details: JobDetails) {
    context.startActivity(Intent(Intent.ACTION_VIEW, details.job.url.toUri()))
}

private fun sharePosting(context: Context, details: JobDetails) {
    val send = Intent(Intent.ACTION_SEND).setType("text/plain")
        .putExtra(Intent.EXTRA_SUBJECT, listOfNotNull(details.job.title, details.companyName).joinToString(" · "))
        .putExtra(Intent.EXTRA_TEXT, details.job.url)
    context.startActivity(Intent.createChooser(send, null))
}

/** A fact as text; nil when the posting doesn't say. */
private fun describe(value: kotlinx.serialization.json.JsonElement): String? = when (value) {
    is JsonNull -> null
    is JsonPrimitive -> value.contentOrNull?.takeIf { it.isNotBlank() && !it.equals("not stated", ignoreCase = true) }
    is JsonArray -> value.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.filter { it.isNotBlank() }.joinToString(", ").takeIf { it.isNotEmpty() }
    else -> null
}
