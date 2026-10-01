package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
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
import com.tonypine.jobsearchhub.core.DecisionQueueItem

/** The briefed jobs waiting for a decision, best match first; a job opens on its brief. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DecideScreen(state: HubState, onRefresh: () -> Unit, onOpenJob: (String) -> Unit) {
    Column {
        TopAppBar(title = { Text("Decide") })
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.error?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) } }
                item {
                    Text(
                        if (state.decisionQueue.size == 1) "1 job to decide" else "${state.decisionQueue.size} jobs to decide",
                        style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                    )
                }
                items(state.decisionQueue, key = { it.job.id }) { item ->
                    DecisionQueueRow(item) { onOpenJob(item.job.id) }
                    HorizontalDivider()
                }
            }
        }
    }
}

@Composable
private fun DecisionQueueRow(item: DecisionQueueItem, onOpen: () -> Unit) {
    Column(Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            MatchLabel(item.match)
            Text(item.job.title, maxLines = 1, modifier = Modifier.weight(1f))
            if (item.decision != null) Text("Later", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        item.companyName?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        Text(item.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2)
    }
}

/** A brief's match as a colored word. */
@Composable
fun MatchLabel(match: String) {
    val color = when (match) {
        "strong" -> Color(0xFF2E9E4F)
        "possible" -> Color(0xFF3B7DD8)
        "stretch" -> Color(0xFFD08A00)
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(match.replaceFirstChar { it.uppercase() }, color = color, style = MaterialTheme.typography.labelLarge)
}
