package com.tonypine.jobsearchhub.ui

import android.content.Intent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.CompanyBrief
import com.tonypine.jobsearchhub.HubViewModel

/** A company's brief, for before an interview. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CompanyScreen(id: String, viewModel: HubViewModel, onBack: () -> Unit, onOpenJob: (String) -> Unit) {
    var brief by remember { mutableStateOf<CompanyBrief?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(id) {
        viewModel.loadCompanyBrief(id).onSuccess { brief = it }.onFailure { error = it.message }
    }
    Column(Modifier.fillMaxSize()) {
        TopAppBar(
            title = { Text(brief?.dossier?.company?.name ?: "Company") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back") } },
        )
        when {
            brief != null -> BriefView(brief!!, onOpenJob)
            error != null -> Text(error!!, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp))
            else -> CircularProgressIndicator(Modifier.padding(24.dp).align(Alignment.CenterHorizontally))
        }
    }
}

@Composable
private fun BriefView(brief: CompanyBrief, onOpenJob: (String) -> Unit) {
    val context = LocalContext.current
    val company = brief.dossier.company
    Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text(company.name, style = MaterialTheme.typography.titleLarge)
        Text(listOfNotNull(company.domain, company.headquartersCountry).joinToString(" · "), color = MaterialTheme.colorScheme.onSurfaceVariant)
        company.summary?.let { Text(it) }
        company.careersUrl?.let { url ->
            TextButton(onClick = { context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri())) }) { Text("Careers page") }
        }

        if (brief.cards.isNotEmpty()) {
            Heading("On your pipeline")
            brief.cards.forEach { (card, phase) ->
                Text("${card.jobTitle ?: company.name}: $phase", fontWeight = FontWeight.Medium)
                card.application.notes?.takeIf { it.isNotBlank() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                card.application.closedReason?.takeIf { it.isNotBlank() }?.let { Text("Closed: $it", style = MaterialTheme.typography.bodyMedium) }
            }
        }

        brief.dossier.connections?.takeIf { it.isNotEmpty() }?.let { connections ->
            Heading("People you know there")
            connections.forEach { person ->
                Text("${person.firstName} ${person.lastName}" + (person.position?.let { " · $it" } ?: ""))
            }
        }

        brief.dossier.warmPaths?.takeIf { it.isNotEmpty() }?.let { paths ->
            Heading("Can introduce you")
            paths.forEach { path ->
                Column {
                    Text(path.name, fontWeight = FontWeight.Medium)
                    Text(
                        listOfNotNull(path.note, path.howKnown, path.preferredChannel?.let { "prefers $it" }).filter { it.isNotBlank() }.joinToString(" · "),
                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }

        if (brief.dossier.people.isNotEmpty()) {
            Heading("People")
            brief.dossier.people.forEach { person ->
                Text(person.name + (person.roleTitle?.let { " · $it" } ?: ""))
            }
        }

        Heading(if (brief.openJobs.size == 1) "1 open role" else "${brief.openJobs.size} open roles")
        brief.openJobs.forEach { item ->
            Row(Modifier.fillMaxWidth().clickable { onOpenJob(item.job.id) }.padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(item.job.title)
                    item.job.location?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
                FitLabel(item.fit.level)
            }
        }
    }
}

@Composable
private fun Heading(text: String) {
    Text(text, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 6.dp))
}
