package com.tonypine.jobsearchhub.ui

import android.content.Intent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.core.net.toUri
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.HubUpdate
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.Spacing

@OptIn(ExperimentalMaterial3Api::class)
@Composable
/** Every update, newest first: the history behind Today's Updates. */
fun UpdatesScreen(state: HubState, onBack: () -> Unit, onRefresh: () -> Unit, onSeen: (HubUpdate) -> Unit, onOpenJob: (String) -> Unit, onOpenCompany: (String) -> Unit) {
    val context = LocalContext.current
    val listState = rememberLazyListState()
    // A keyed list keeps its first row in place, which would leave newer updates just above the top.
    LaunchedEffect(state.updates.firstOrNull()?.id) {
        listState.scrollToItem(0)
    }
    Column {
        TopAppBar(
            title = { Text("Updates") },
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back") } },
        )
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize(), state = listState) {
                state.error?.let { item { HubErrorView("Couldn't reach the hub", it, onRetry = onRefresh) } }
                if (state.updates.isEmpty() && !state.isLoading && state.error == null) {
                    item { Text("No updates yet.", modifier = Modifier.padding(Spacing.l)) }
                }
                items(state.updates, key = { it.id }) { update ->
                    UpdateRow(update) {
                        onSeen(update)
                        when {
                            update.jobId != null -> onOpenJob(update.jobId!!)
                            update.companyId != null -> onOpenCompany(update.companyId!!)
                            update.sourceUrl != null -> context.startActivity(Intent(Intent.ACTION_VIEW, update.sourceUrl!!.toUri()))
                        }
                    }
                    HorizontalDivider()
                }
            }
        }
    }
}

@Composable
private fun UpdateRow(update: HubUpdate, onOpen: () -> Unit) {
    Column(Modifier.clickable(onClick = onOpen).padding(horizontal = Spacing.l, vertical = Spacing.m)) {
        Text(update.title, fontWeight = if (update.seenAt == null) FontWeight.Bold else FontWeight.Normal)
        listOfNotNull(update.companyName, update.jobTitle).joinToString(" · ").takeIf { it.isNotEmpty() }?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
        }
        if (update.body.isNotBlank()) {
            Text(update.body, style = MaterialTheme.typography.bodyMedium, maxLines = 3)
        }
        Text(formatWhen(update.createdAt), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
