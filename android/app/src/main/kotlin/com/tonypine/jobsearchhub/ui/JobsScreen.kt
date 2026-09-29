package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import kotlinx.coroutines.launch
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.JobListItem

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobsScreen(
    state: HubState, onRefresh: () -> Unit, onIncludeUnclear: (Boolean) -> Unit, onOpenJob: (String) -> Unit,
    onResearch: suspend (String) -> String?,
) {
    var isAsking by remember { mutableStateOf(false) }
    var company by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    if (isAsking) {
        AlertDialog(
            onDismissRequest = { isAsking = false },
            title = { Text("Research a company") },
            text = {
                Column {
                    Text("The Mac researches it, puts it on the watch list and finds its jobs.")
                    OutlinedTextField(company, { company = it }, label = { Text("Name or link") })
                }
            },
            confirmButton = {
                TextButton(enabled = company.isNotBlank(), onClick = {
                    isAsking = false
                    scope.launch { message = onResearch(company.trim()); company = "" }
                }) { Text("Ask the Mac") }
            },
            dismissButton = { TextButton(onClick = { isAsking = false }) { Text("Cancel") } },
        )
    }
    Column {
        TopAppBar(title = { Text("Jobs") }, actions = {
            IconButton(onClick = { isAsking = true }) { Icon(Icons.Filled.Add, contentDescription = "Research a company") }
            FilterChip(selected = state.includesUnclear, onClick = { onIncludeUnclear(!state.includesUnclear) }, label = { Text("Unclear too") })
            Spacer(Modifier.width(8.dp))
        })
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.error?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) } }
                message?.let { item { Text(it, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(horizontal = 16.dp)) } }
                item {
                    Text(
                        "${state.shownJobs.size} of ${state.openJobCount} open jobs",
                        style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                    )
                }
                items(state.shownJobs, key = { it.job.id }) { item ->
                    JobRow(item) { onOpenJob(item.job.id) }
                    HorizontalDivider()
                }
            }
        }
    }
}

@Composable
private fun JobRow(item: JobListItem, onOpen: () -> Unit) {
    Row(Modifier.clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(item.job.title, maxLines = 2)
            Text(
                listOfNotNull(item.companyName, item.job.location).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1,
            )
        }
        Spacer(Modifier.width(8.dp))
        FitLabel(item.fit.level)
    }
}

@Composable
fun FitLabel(level: String) {
    val color = when (level) {
        "good" -> Color(0xFF2E9E4F)
        "poor" -> MaterialTheme.colorScheme.error
        else -> Color(0xFFD08A00)
    }
    Text(level.replaceFirstChar { it.uppercase() }, color = color, style = MaterialTheme.typography.labelLarge)
}
