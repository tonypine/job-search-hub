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
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.DecisionQueueItem
import com.tonypine.jobsearchhub.core.Match
import com.tonypine.jobsearchhub.core.Tone
import com.tonypine.jobsearchhub.ui.design.HubAction
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.OverflowMenu
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip

/** The briefed jobs waiting for a decision, best match first; a job opens on its brief. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DecideScreen(state: HubState, onRefresh: () -> Unit, onOpenJob: (String) -> Unit, menu: List<HubAction>, selected: Detail? = null) {
    Column {
        TopAppBar(title = { Text("Decide") }, actions = { OverflowMenu(menu) })
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.error?.let { item { HubErrorView("Couldn't reach the hub", it, onRetry = onRefresh) } }
                item {
                    Text(
                        if (state.decisionQueue.size == 1) "1 job to decide" else "${state.decisionQueue.size} jobs to decide",
                        style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = Spacing.l, vertical = Spacing.s),
                    )
                }
                items(state.decisionQueue, key = { it.job.id }) { item ->
                    DecisionQueueRow(item, isSelected = Detail.job(item.job.id).isSameItem(selected)) { onOpenJob(item.job.id) }
                    HorizontalDivider()
                }
            }
        }
    }
}

@Composable
private fun DecisionQueueRow(item: DecisionQueueItem, isSelected: Boolean, onOpen: () -> Unit) {
    Column(Modifier.fillMaxWidth().selectedBackground(isSelected, MaterialTheme.colorScheme.primaryContainer).clickable(onClick = onOpen).padding(horizontal = Spacing.l, vertical = Spacing.m), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Spacing.s)) {
            val match = Match.of(item.match)
            ToneChip(match.word, match.tone)
            Text(item.job.title, maxLines = 1, modifier = Modifier.weight(1f))
            if (item.decision != null) ToneChip("Later", Tone.NEUTRAL)
        }
        item.companyName?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        Text(item.reason, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2)
    }
}
