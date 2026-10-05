package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.rounded.Done
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.HubState
import com.tonypine.jobsearchhub.PipelineFocus
import com.tonypine.jobsearchhub.core.PipelineBoard
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.core.PipelinePhase
import com.tonypine.jobsearchhub.core.Tone
import com.tonypine.jobsearchhub.ui.design.HubErrorView
import com.tonypine.jobsearchhub.ui.design.HubTheme
import com.tonypine.jobsearchhub.ui.design.Monogram
import com.tonypine.jobsearchhub.ui.design.Spacing
import com.tonypine.jobsearchhub.ui.design.ToneChip
import com.tonypine.jobsearchhub.ui.design.segmentShape
import java.time.Instant
import java.time.ZoneId

/** The applications by phase: a row of phase chips, and the chosen phase's cards, each with *Followed up…* and *Move to*. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PipelineScreen(
    state: HubState,
    focus: PipelineFocus?,
    onFocusShown: () -> Unit,
    onRefresh: () -> Unit,
    onOpenCard: (PipelineCard) -> Unit,
    onFollowedUp: (PipelineCard, String) -> Unit,
    onMove: (PipelineCard, PipelinePhase, String) -> Unit,
    onOpenSettings: () -> Unit,
    selected: Detail? = null,
) {
    val board = state.pipeline
    var chosenPhaseId by rememberSaveable { mutableStateOf<String?>(null) }
    var highlightedCardId by rememberSaveable { mutableStateOf<String?>(null) }
    var followingUp by remember { mutableStateOf<PipelineCard?>(null) }
    var closing by remember { mutableStateOf<Pair<PipelineCard, PipelinePhase>?>(null) }
    val phases = board?.orderedPhases.orEmpty()
    val phase = phases.firstOrNull { it.id == chosenPhaseId } ?: board?.let(::firstBusyPhase)
    val cards = if (board != null && phase != null) board.cardsIn(phase.id) else emptyList()
    val chipsState = rememberLazyListState()
    val cardsState = rememberLazyListState()
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()

    // A follow-up reminder opens its card: its phase chosen, the card in view and marked.
    LaunchedEffect(focus, board) {
        if (focus == null || board == null) return@LaunchedEffect
        board.findCard(focus.jobId, focus.companyId)?.let { card ->
            chosenPhaseId = card.application.phaseId
            highlightedCardId = card.id
            chipsState.animateScrollToItem(board.orderedPhases.indexOfFirst { it.id == card.application.phaseId }.coerceAtLeast(0))
            // The error, when there is one, is the list's first item.
            val leading = if (state.error != null) 1 else 0
            cardsState.animateScrollToItem(leading + board.cardsIn(card.application.phaseId).indexOfFirst { it.id == card.id }.coerceAtLeast(0))
        }
        onFocusShown()
    }

    followingUp?.let { card ->
        FollowedUpSheet(card, board?.phaseOf(card), onDismiss = { followingUp = null }) { note ->
            followingUp = null
            onFollowedUp(card, note)
        }
    }
    closing?.let { (card, closedPhase) ->
        CloseDialog(card, closedPhase, onDismiss = { closing = null }) { reason ->
            closing = null
            onMove(card, closedPhase, reason)
        }
    }

    Column(Modifier.nestedScroll(scrollBehavior.nestedScrollConnection)) {
        PageTopAppBar("Pipeline", state, scrollBehavior, onOpenSettings)
        board?.let { PageSummary(it.summary(Instant.now(), ZoneId.systemDefault()), Modifier.padding(bottom = Spacing.s)) }
        LazyRow(
            state = chipsState, contentPadding = PaddingValues(horizontal = Spacing.l),
            horizontalArrangement = Arrangement.spacedBy(Spacing.s),
        ) {
            items(phases, key = { it.id }) { item ->
                FilterChip(
                    selected = item.id == phase?.id, onClick = { chosenPhaseId = item.id; highlightedCardId = null },
                    label = { Text("${item.name} ${board?.cardsIn(item.id)?.size ?: 0}") },
                )
            }
        }
        PullToRefreshBox(isRefreshing = state.isLoading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(
                Modifier.fillMaxSize(), state = cardsState,
                contentPadding = PaddingValues(Spacing.l), verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                state.error?.let { item { HubErrorView("Couldn't reach the hub", it, onRetry = onRefresh) } }
                when {
                    board == null -> Unit
                    phases.isEmpty() -> item { EmptyLine("The pipeline has no phases yet. Add them in the Mac app's Settings.") }
                    cards.isEmpty() -> item { EmptyLine("Nothing in ${phase?.name}.") }
                }
                if (board != null) itemsIndexed(cards, key = { _, card -> card.id }) { index, card ->
                    PipelineCardRow(
                        card, board, isHighlighted = card.id == highlightedCardId || card.detail()?.isSameItem(selected) == true,
                        modifier = Modifier.clip(segmentShape(index, cards.size)),
                        onOpen = { onOpenCard(card) },
                        onFollowedUp = { followingUp = card },
                        onMove = { target -> if (target.isClosed) closing = card to target else onMove(card, target, "") },
                    )
                }
            }
        }
    }
}

/** The phase a board opens on: the first open one with cards, or else the first. */
private fun firstBusyPhase(board: PipelineBoard): PipelinePhase? =
    board.orderedPhases.firstOrNull { !it.isClosed && board.cardsIn(it.id).isNotEmpty() } ?: board.orderedPhases.firstOrNull()

@Composable
private fun PipelineCardRow(
    card: PipelineCard,
    board: PipelineBoard,
    isHighlighted: Boolean,
    modifier: Modifier = Modifier,
    onOpen: () -> Unit,
    onFollowedUp: () -> Unit,
    onMove: (PipelinePhase) -> Unit,
) {
    val now = Instant.now()
    val followUp = card.followUpStatus(now, ZoneId.systemDefault())
    val background = if (isHighlighted) MaterialTheme.colorScheme.primaryContainer else HubTheme.colors.item
    Row(
        modifier.fillMaxWidth().background(background).clickable(onClick = onOpen).heightIn(min = 72.dp)
            .padding(start = Spacing.l, top = Spacing.m, bottom = Spacing.m),
        horizontalArrangement = Arrangement.spacedBy(Spacing.l),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Monogram(card.companyName ?: card.title)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(Spacing.xs)) {
            Text(card.title, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
                followUp?.let { ToneChip(it.label, it.tone) }
                if (card.isHeardBack) ToneChip("Heard back", Tone.POSITIVE)
                Text(
                    listOfNotNull(card.companyName.takeIf { card.jobTitle != null }, dayCount(card.daysInPhase(now))).joinToString(" · "),
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
        }
        CardMenu(card, board, onFollowedUp, onMove)
    }
}

/** A card's `⋮` menu: *Followed up…* on an open card, and *Move to*, which lists the other phases. */
@Composable
private fun CardMenu(card: PipelineCard, board: PipelineBoard, onFollowedUp: () -> Unit, onMove: (PipelinePhase) -> Unit) {
    var isOpen by remember { mutableStateOf(false) }
    var showsPhases by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { isOpen = true; showsPhases = false }) { Icon(Icons.Rounded.MoreVert, contentDescription = "Actions for ${card.title}") }
        DropdownMenu(expanded = isOpen, onDismissRequest = { isOpen = false }) {
            if (showsPhases) {
                DropdownMenuItem(
                    text = { Text("Move to") }, leadingIcon = { Icon(Icons.AutoMirrored.Rounded.ArrowBack, contentDescription = "Back") },
                    onClick = { showsPhases = false },
                )
                board.orderedPhases.filter { it.id != card.application.phaseId }.forEach { phase ->
                    DropdownMenuItem(text = { Text(if (phase.isClosed) "${phase.name}…" else phase.name) }, onClick = {
                        isOpen = false
                        onMove(phase)
                    })
                }
            } else {
                if (board.phaseOf(card)?.isClosed != true) {
                    DropdownMenuItem(
                        text = { Text("Followed up…") }, leadingIcon = { Icon(Icons.Rounded.Done, contentDescription = null) },
                        onClick = { isOpen = false; onFollowedUp() },
                    )
                }
                DropdownMenuItem(
                    text = { Text("Move to") }, leadingIcon = { Icon(Icons.AutoMirrored.Rounded.ArrowForward, contentDescription = null) },
                    onClick = { showsPhases = true },
                )
            }
        }
    }
}

/** Moving a card to a closed phase ends the application, with the reason it ended. */
@Composable
private fun CloseDialog(card: PipelineCard, phase: PipelinePhase, onDismiss: () -> Unit, onClose: (String) -> Unit) {
    var reason by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Move ${card.title} to ${phase.name}?") },
        text = { OutlinedTextField(reason, { reason = it }, label = { Text("Why it ended") }) },
        confirmButton = { TextButton(onClick = { onClose(reason) }) { Text("Move") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun EmptyLine(text: String) {
    Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = Spacing.l))
}
