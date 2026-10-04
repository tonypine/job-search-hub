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
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.JobListItem
import com.tonypine.jobsearchhub.core.Screen
import com.tonypine.jobsearchhub.ui.design.HubAction
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.OverflowMenu
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun JobsScreen(state: HubState, onRefresh: () -> Unit, onIncludeUnclear: (Boolean) -> Unit, onOpenJob: (String) -> Unit, menu: List<HubAction>) {
    Column {
        TopAppBar(title = { Text("Jobs") }, actions = {
            FilterChip(selected = state.includesUnclear, onClick = { onIncludeUnclear(!state.includesUnclear) }, label = { Text("Unclear too") })
            OverflowMenu(menu)
        })
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.error?.let { item { HubErrorView("Couldn't reach the hub", it, onRetry = onRefresh) } }
                item {
                    Text(
                        "${state.shownJobs.size} of ${state.openJobCount} open jobs",
                        style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = Spacing.l, vertical = Spacing.s),
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
    Row(Modifier.clickable(onClick = onOpen).padding(horizontal = Spacing.l, vertical = Spacing.m), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(item.job.title, maxLines = 2)
            Text(
                listOfNotNull(item.companyName, item.job.location).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1,
            )
        }
        Spacer(Modifier.width(Spacing.s))
        val screen = Screen.ofLevel(item.fit.level)
        ToneChip(screen.word, screen.tone)
    }
}
