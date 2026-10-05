package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarScrollBehavior
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.ui.design.Monogram
import com.tonypine.jobsearchhub.ui.design.Spacing

/**
 * A top-level page's large app bar, which collapses to a small one on scroll:
 * the page's name, its [actions], and last the hub avatar, which opens Settings.
 * Material3 1.4's flexible bars, with a subtitle, are still experimental, so the
 * summary goes under the bar as a [PageSummary].
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PageTopAppBar(
    title: String,
    state: HubState,
    scrollBehavior: TopAppBarScrollBehavior,
    onOpenSettings: () -> Unit,
    actions: @Composable RowScope.() -> Unit = {},
) {
    LargeTopAppBar(
        title = { Text(title) }, scrollBehavior = scrollBehavior,
        actions = {
            actions()
            HubAvatar(state, onOpenSettings)
        },
    )
}

/** The hub's monogram, with a small error badge while the phone can't reach it. */
@Composable
private fun HubAvatar(state: HubState, onClick: () -> Unit) {
    val isConnected = state.error == null
    IconButton(onClick = onClick, modifier = Modifier.semantics { contentDescription = if (isConnected) "Settings, hub connected" else "Settings, hub disconnected" }) {
        BadgedBox(badge = { if (!isConnected) Badge() }, modifier = Modifier.clearAndSetSemantics {}) {
            Monogram(state.pairing?.hubName ?: "Hub", size = 32.dp)
        }
    }
}

/** The line under a top-level page's title that sums the page up: "7 to decide". */
@Composable
fun PageSummary(text: String, modifier: Modifier = Modifier) {
    Text(
        text, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier.padding(horizontal = Spacing.l),
    )
}

/** A job's or company's name as its medium app bar's title, on one line, and a heading for TalkBack. */
@Composable
fun EntityTitle(name: String) {
    Text(name, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.semantics { heading() })
}
