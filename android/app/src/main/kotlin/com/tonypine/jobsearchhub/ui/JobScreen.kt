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
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.core.JobDetails
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobScreen(id: String, viewModel: HubViewModel, onBack: () -> Unit, onOpenCompany: (String) -> Unit) {
    var details by remember { mutableStateOf<JobDetails?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(id) {
        viewModel.loadJob(id).onSuccess { details = it }.onFailure { error = it.message }
    }
    Column(Modifier.fillMaxSize()) {
        TopAppBar(
            title = { Text(details?.companyName ?: "Job") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back") } },
        )
        when {
            details != null -> JobDetailsView(details!!, onOpenCompany)
            error != null -> Text(error!!, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp))
            else -> CircularProgressIndicator(Modifier.padding(24.dp).align(Alignment.CenterHorizontally))
        }
    }
}

@Composable
private fun JobDetailsView(details: JobDetails, onOpenCompany: (String) -> Unit) {
    val context = LocalContext.current
    Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(details.job.title, style = MaterialTheme.typography.titleLarge)
        Text(listOfNotNull(details.companyName, details.job.location, details.job.workplaceType).joinToString(" · "), color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { context.startActivity(Intent(Intent.ACTION_VIEW, details.job.url.toUri())) }) { Text("Open posting") }
            details.job.companyId?.let { companyId ->
                OutlinedButton(onClick = { onOpenCompany(companyId) }) { Text("Company brief") }
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
