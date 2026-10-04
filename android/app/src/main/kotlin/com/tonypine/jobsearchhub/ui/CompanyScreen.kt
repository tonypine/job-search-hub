package com.tonypine.jobsearchhub.ui

import android.content.Intent
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
import androidx.compose.material.icons.automirrored.rounded.OpenInNew
import androidx.compose.material.icons.rounded.TravelExplore
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.CompanyBrief
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.core.Screen
import com.tonypine.jobsearchhub.ui.design.EntityHeader
import com.tonypine.jobsearchhub.ui.design.HubAction
import com.tonypine.jobsearchhub.ui.design.HubCard
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.HubSection
import com.tonypine.jobsearchhub.ui.design.OverflowMenu
import com.tonypine.jobsearchhub.ui.design.PersonRow
import com.tonypine.jobsearchhub.ui.design.Relation
import com.tonypine.jobsearchhub.ui.design.SegmentedGroup
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip
import kotlinx.coroutines.launch

/** A company's brief, for before an interview. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CompanyScreen(id: String, viewModel: HubViewModel, onBack: () -> Unit, onOpenJob: (String) -> Unit) {
    val context = LocalContext.current
    var brief by remember { mutableStateOf<CompanyBrief?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var attempt by remember { mutableIntStateOf(0) }
    var message by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(id, attempt) {
        error = null
        viewModel.loadCompanyBrief(id).onSuccess { brief = it }.onFailure { error = it.message ?: it.toString() }
    }
    Column(Modifier.fillMaxSize()) {
        TopAppBar(
            title = {},
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back") } },
            actions = {
                brief?.dossier?.company?.careersUrl?.let { url ->
                    OverflowMenu(
                        listOf(
                            HubAction("Careers page", Icons.AutoMirrored.Rounded.OpenInNew) {
                                context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
                            },
                        ),
                    )
                }
            },
        )
        val shown = brief
        when {
            shown != null -> BriefView(shown, message, onOpenJob, onFindJobs = {
                scope.launch {
                    message = viewModel.askTheMac(QueueTaskRequest(kind = "find_jobs", companyId = id))
                        .fold({ "Sent to the Mac. The result comes as an update." }, { it.message })
                }
            })
            error != null -> HubErrorView("Couldn't load this company", error, onRetry = { attempt++ })
            else -> CircularProgressIndicator(Modifier.padding(Spacing.xl).align(Alignment.CenterHorizontally))
        }
    }
}

@Composable
private fun BriefView(brief: CompanyBrief, message: String?, onOpenJob: (String) -> Unit, onFindJobs: () -> Unit) {
    val company = brief.dossier.company
    Column(Modifier.verticalScroll(rememberScrollState()).padding(bottom = Spacing.l), verticalArrangement = Arrangement.spacedBy(Spacing.l)) {
        EntityHeader(company.name, facts = listOfNotNull(company.domain, company.headquartersCountry).joinToString(" · "))
        Column(Modifier.padding(horizontal = Spacing.m), verticalArrangement = Arrangement.spacedBy(Spacing.l)) {
            company.summary?.takeIf { it.isNotBlank() }?.let {
                HubSection("Summary") { HubCard { Text(it, style = MaterialTheme.typography.bodyMedium) } }
            }
            if (brief.cards.isNotEmpty()) {
                HubSection("On your pipeline") {
                    HubCard {
                        brief.cards.forEach { (card, phase) ->
                            Column {
                                Text("${card.jobTitle ?: company.name}: $phase", style = MaterialTheme.typography.bodyLarge)
                                card.application.notes?.takeIf { it.isNotBlank() }?.let {
                                    Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                card.application.closedReason?.takeIf { it.isNotBlank() }?.let {
                                    Text("Closed: $it", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                        }
                    }
                }
            }
            PeopleSection(brief)
            HubSection(
                if (brief.openJobs.size == 1) "1 open role" else "${brief.openJobs.size} open roles",
                trailing = { TextButton(onClick = onFindJobs) { Icon(Icons.Rounded.TravelExplore, null); Text("  Find jobs on the Mac") } },
            ) {
                message?.let {
                    Text(
                        it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(horizontal = Spacing.l, vertical = Spacing.s),
                    )
                }
                SegmentedGroup(
                    brief.openJobs.map { item ->
                        {
                            Row(
                                Modifier.fillMaxWidth().clickable { onOpenJob(item.job.id) }.heightIn(min = 56.dp)
                                    .padding(horizontal = Spacing.l, vertical = Spacing.m),
                                horizontalArrangement = Arrangement.spacedBy(Spacing.l),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(item.job.title, style = MaterialTheme.typography.bodyLarge)
                                    item.job.location?.let {
                                        Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                }
                                val screen = Screen.ofLevel(item.fit.level)
                                ToneChip(screen.word, screen.tone)
                            }
                        }
                    },
                )
            }
        }
    }
}

/** Everyone who can get the owner in, in one group: connections, introducers and contacts. */
@Composable
private fun PeopleSection(brief: CompanyBrief) {
    val dossier = brief.dossier
    val connections = dossier.connections.orEmpty().map { person ->
        @Composable { PersonRow("${person.firstName} ${person.lastName}".trim(), Relation.CONNECTION, role = person.position) }
    }
    val introducers = dossier.warmPaths.orEmpty().map { path ->
        @Composable {
            PersonRow(
                path.name, Relation.INTRODUCER,
                role = listOfNotNull(path.note, path.howKnown, path.preferredChannel?.let { "prefers $it" })
                    .filter { it.isNotBlank() }.joinToString(" · "),
            )
        }
    }
    val contacts = dossier.people.map { person -> @Composable { PersonRow(person.name, Relation.CONTACT, role = person.roleTitle) } }
    val rows = connections + introducers + contacts
    if (rows.isEmpty()) return
    HubSection("People") { SegmentedGroup(rows) }
}
