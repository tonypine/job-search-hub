package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Done
import androidx.compose.material.icons.rounded.Laptop
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.core.HubUpdate
import com.tonypine.jobsearchhub.core.Match
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.core.Today
import com.tonypine.jobsearchhub.core.Tone
import com.tonypine.jobsearchhub.ui.design.HubAction
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.HubSection
import com.tonypine.jobsearchhub.ui.design.Monogram
import com.tonypine.jobsearchhub.ui.design.OverflowMenu
import com.tonypine.jobsearchhub.ui.design.SegmentedGroup
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip
import com.tonypine.jobsearchhub.versions.VersionState

/** How many of a source's items Today shows before *See all*. */
private const val SHOWN = 3

/**
 * What needs you now: a group for each source that has something, the jobs to
 * decide, the follow-ups due, the unseen updates and the recruiters waiting,
 * under a line that sums them up.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TodayScreen(
    state: HubState,
    onRefresh: () -> Unit,
    onOpenDecisionJob: (String) -> Unit,
    onSeeDecide: () -> Unit,
    onOpenCard: (PipelineCard) -> Unit,
    onSeePipeline: () -> Unit,
    onFollowedUp: (PipelineCard, String) -> Unit,
    onOpenUpdate: (HubUpdate) -> Unit,
    onSeeUpdates: () -> Unit,
    onOpenCompany: (String) -> Unit,
    onAskTheMac: (String) -> Unit,
    onOpenSettings: () -> Unit,
    version: VersionState,
    versionActions: VersionActions,
    selected: Detail? = null,
) {
    var isAsking by rememberSaveable { mutableStateOf(false) }
    var followingUp by remember { mutableStateOf<PipelineCard?>(null) }
    val followUps = state.dueFollowUps()
    val updates = Today.unseenUpdates(state.updates)
    val recruiters = state.recruiters.filter { it.isWaiting }
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()

    if (isAsking) {
        AskTheMacDialog(onDismiss = { isAsking = false }) { company ->
            isAsking = false
            onAskTheMac(company)
        }
    }
    followingUp?.let { card ->
        FollowedUpSheet(card, state.pipeline?.phaseOf(card), onDismiss = { followingUp = null }) { note ->
            followingUp = null
            onFollowedUp(card, note)
        }
    }

    Column(Modifier.nestedScroll(scrollBehavior.nestedScrollConnection)) {
        PageTopAppBar("Today", state, scrollBehavior, onOpenSettings) {
            OverflowMenu(listOf(HubAction("Ask the Mac…", Icons.Rounded.Laptop) { isAsking = true }))
        }
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(Spacing.s)) {
                if (version.showsCard()) item { NewVersionCard(version, versionActions) }
                item { PageSummary(Today.summary(state.decisionQueue.size, followUps.map { it.second }, state.updates)) }
                state.error?.let { item { HubErrorView("Couldn't reach the hub", it, onRetry = onRefresh) } }
                if (state.decisionQueue.isNotEmpty()) item {
                    TodayGroup("Decide · ${state.decisionQueue.size}", onSeeAll = onSeeDecide, rows = state.decisionQueue.take(SHOWN).map { item ->
                        {
                            val match = Match.of(item.match)
                            TodayRow(
                                item.companyName ?: item.job.title, item.job.title, onOpen = { onOpenDecisionJob(item.job.id) },
                                isSelected = Detail.job(item.job.id).isSameItem(selected),
                            ) {
                                ToneChip(match.word, match.tone)
                                Supporting(listOfNotNull(item.companyName, item.job.location).joinToString(" · "))
                            }
                        }
                    })
                }
                if (followUps.isNotEmpty()) item {
                    TodayGroup("Follow up · ${followUps.size}", onSeeAll = onSeePipeline, rows = followUps.take(SHOWN).map { (card, status) ->
                        {
                            TodayRow(
                                card.companyName ?: card.title, card.title, onOpen = { onOpenCard(card) },
                                isSelected = card.detail()?.isSameItem(selected) == true,
                                trailing = {
                                    FilledTonalIconButton(onClick = { followingUp = card }) {
                                        Icon(Icons.Rounded.Done, contentDescription = "Followed up on ${card.title}…")
                                    }
                                },
                            ) {
                                ToneChip(status.label, status.tone)
                                card.companyName?.takeIf { card.jobTitle != null }?.let { Supporting(it) }
                            }
                        }
                    })
                }
                if (updates.isNotEmpty()) item {
                    TodayGroup("Updates · ${updates.size}", onSeeAll = onSeeUpdates, rows = updates.take(SHOWN).map { update ->
                        {
                            TodayRow(
                                update.companyName ?: update.title, update.title, onOpen = { onOpenUpdate(update) },
                                isSelected = update.detail()?.isSameItem(selected) == true,
                                trailing = { Supporting(formatWhen(update.createdAt)) },
                            ) {
                                Supporting(update.body.lineSequence().firstOrNull { it.isNotBlank() } ?: listOfNotNull(update.companyName, update.jobTitle).joinToString(" · "))
                            }
                        }
                    })
                }
                if (recruiters.isNotEmpty()) item {
                    TodayGroup("Recruiters waiting · ${recruiters.size}", rows = recruiters.take(SHOWN).map { recruiter ->
                        {
                            TodayRow(
                                recruiter.startedByName, recruiter.startedByName.ifBlank { "A recruiter" }, tone = Tone.NEUTRAL,
                                onOpen = recruiter.companyId?.let { id -> { onOpenCompany(id) } },
                                isSelected = recruiter.companyId?.let { Detail.company(it).isSameItem(selected) } == true,
                            ) {
                                if (recruiter.isAgency) ToneChip("Agency", Tone.NEUTRAL)
                                val fitting = if (recruiter.fittingJobs == 1) "1 fitting job" else "${recruiter.fittingJobs} fitting jobs"
                                Supporting(listOfNotNull(recruiter.hiringCompany, fitting).joinToString(" · "))
                            }
                        }
                    })
                }
                if (state.error == null && !state.isLoading && state.decisionQueue.isEmpty() && followUps.isEmpty() && updates.isEmpty() && recruiters.isEmpty()) {
                    item {
                        Text(
                            "You're all caught up. New matches, replies and follow-ups show here.",
                            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(Spacing.l),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun TodayGroup(title: String, rows: List<@Composable () -> Unit>, onSeeAll: (() -> Unit)? = null) {
    HubSection(title, trailing = onSeeAll?.let { { TextButton(onClick = it) { Text("See all") } } }) {
        SegmentedGroup(rows, Modifier.padding(horizontal = Spacing.l))
    }
}

/** A list item: a monogram, the headline, a line of chips and supporting text, and an optional trailing element. Selected while its item is open beside the list. */
@Composable
private fun TodayRow(
    monogram: String,
    headline: String,
    onOpen: (() -> Unit)?,
    tone: Tone = Tone.ACCENT,
    isSelected: Boolean = false,
    trailing: (@Composable () -> Unit)? = null,
    supporting: @Composable RowScope.() -> Unit,
) {
    Row(
        Modifier.fillMaxWidth().selectedBackground(isSelected, MaterialTheme.colorScheme.primaryContainer).then(if (onOpen != null) Modifier.clickable(onClick = onOpen) else Modifier).heightIn(min = 72.dp)
            .padding(horizontal = Spacing.l, vertical = Spacing.m),
        horizontalArrangement = Arrangement.spacedBy(Spacing.l),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Monogram(monogram, tone = tone)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(Spacing.xs)) {
            Text(headline, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically, content = supporting)
        }
        trailing?.invoke()
    }
}

@Composable
private fun Supporting(text: String) {
    Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
}

/** Asks the Mac to research a company; it puts it on the watch list and finds its jobs. */
@Composable
private fun AskTheMacDialog(onDismiss: () -> Unit, onAsk: (String) -> Unit) {
    var company by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Rounded.Laptop, contentDescription = null) },
        title = { Text("Ask the Mac") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(Spacing.s)) {
                Text("The Mac researches a company, puts it on the watch list and finds its jobs.")
                OutlinedTextField(company, { company = it }, label = { Text("Company name or link") })
            }
        },
        confirmButton = { TextButton(enabled = company.isNotBlank(), onClick = { onAsk(company.trim()) }) { Text("Research") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
