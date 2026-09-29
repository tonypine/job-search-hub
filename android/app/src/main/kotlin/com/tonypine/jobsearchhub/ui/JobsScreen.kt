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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.JobListItem

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobsScreen(state: HubState, onRefresh: () -> Unit, onIncludeUnclear: (Boolean) -> Unit, onOpenJob: (String) -> Unit) {
    Column {
        TopAppBar(title = { Text("Jobs") }, actions = {
            FilterChip(selected = state.includesUnclear, onClick = { onIncludeUnclear(!state.includesUnclear) }, label = { Text("Unclear too") })
            Spacer(Modifier.width(8.dp))
        })
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.error?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) } }
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
