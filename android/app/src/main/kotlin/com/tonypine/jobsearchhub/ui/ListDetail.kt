package com.tonypine.jobsearchhub.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.ExperimentalMaterial3AdaptiveApi
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.layout.AnimatedPane
import androidx.compose.material3.adaptive.layout.ListDetailPaneScaffold
import androidx.compose.material3.adaptive.layout.ListDetailPaneScaffoldDefaults
import androidx.compose.material3.adaptive.layout.ListDetailPaneScaffoldRole
import androidx.compose.material3.adaptive.layout.PaneAdaptedValue
import androidx.compose.material3.adaptive.layout.PaneExpansionState
import androidx.compose.material3.adaptive.layout.PaneScaffoldDirective
import androidx.compose.material3.adaptive.layout.ThreePaneScaffoldDestinationItem
import androidx.compose.material3.adaptive.layout.ThreePaneScaffoldScope
import androidx.compose.material3.adaptive.layout.calculatePaneScaffoldDirectiveWithTwoPanesOnMediumWidth
import androidx.compose.material3.adaptive.layout.calculateThreePaneScaffoldValue
import androidx.compose.material3.adaptive.layout.defaultDragHandleSemantics
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.core.HubUpdate
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.ui.design.Spacing

/** What a page's detail pane shows: a job, or a company. A job opened from the decision queue moves on to the next one once decided. */
data class Detail(val kind: Kind, val id: String, val fromQueue: Boolean = false) {
    enum class Kind { JOB, COMPANY }

    /** Whether this shows the same job or company as [other], wherever each was opened from. */
    fun isSameItem(other: Detail?): Boolean = other != null && kind == other.kind && id == other.id

    companion object {
        fun job(id: String, fromQueue: Boolean = false) = Detail(Kind.JOB, id, fromQueue)
        fun company(id: String) = Detail(Kind.COMPANY, id)
    }
}

/** A card's job, or its company when it has no job. */
fun PipelineCard.detail(): Detail? = application.jobId?.let { Detail.job(it) } ?: application.companyId?.let { Detail.company(it) }

/** An update's job, or its company when it has no job. */
fun HubUpdate.detail(): Detail? = jobId?.let { Detail.job(it) } ?: companyId?.let { Detail.company(it) }

/**
 * A page's open items, the one shown last: an item opened from the list
 * replaces them, and a link in it (a job's company) goes on top, so back
 * returns to the job. Saved with the page, so rotating or folding keeps it.
 */
@Stable
class DetailStack(items: List<Detail> = emptyList()) {
    var items by mutableStateOf(items)
        private set

    /** The item shown. */
    val current: Detail? get() = items.lastOrNull()

    /** The item opened from the list, which the list highlights. */
    val root: Detail? get() = items.firstOrNull()

    /** Opens an item from the list, in place of what was open. */
    fun show(detail: Detail) {
        items = listOf(detail)
    }

    /** Opens an item from the one shown; back returns to it. */
    fun open(detail: Detail) {
        items = items + detail
    }

    /** Shows [detail] in place of the item shown, like the next job in the queue. */
    fun replace(detail: Detail) {
        items = items.dropLast(1) + detail
    }

    /** Closes the item shown. */
    fun close() {
        items = items.dropLast(1)
    }

    companion object {
        val Saver = listSaver<DetailStack, Any>(
            save = { stack -> stack.items.flatMap { listOf(it.kind.name, it.id, it.fromQueue) } },
            restore = { saved -> DetailStack(saved.chunked(3).map { (kind, id, fromQueue) -> Detail(Detail.Kind.valueOf(kind as String), id as String, fromQueue as Boolean) }) },
        )
    }
}

/** A list row marked while its item is open beside the list, on [color], the scheme's primary container. */
fun Modifier.selectedBackground(isSelected: Boolean, color: Color): Modifier =
    if (isSelected) background(color).semantics { selected = true } else this

@Composable
fun rememberDetailStack(): DetailStack = rememberSaveable(saver = DetailStack.Saver) { DetailStack() }

/**
 * A page as a list and its open item: side by side from 600 dp, with a handle
 * between them to resize the panes, and one at a time below that. Back closes
 * the open item and returns to the list. [detail] gets whether the item shows
 * alone, which is when it needs a back arrow.
 */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
fun ListDetailPage(
    details: DetailStack,
    placeholder: String,
    detail: @Composable (Detail, isAlone: Boolean) -> Unit,
    list: @Composable () -> Unit,
) {
    val directive = paneDirective()
    val current = details.current
    val value = calculateThreePaneScaffoldValue(
        directive.maxHorizontalPartitions, ListDetailPaneScaffoldDefaults.adaptStrategies(),
        ThreePaneScaffoldDestinationItem(if (current != null) ListDetailPaneScaffoldRole.Detail else ListDetailPaneScaffoldRole.List, current),
    )
    val isAlone = value[ListDetailPaneScaffoldRole.List] != PaneAdaptedValue.Expanded
    BackHandler(enabled = current != null) { details.close() }
    ListDetailPaneScaffold(
        directive = directive, value = value,
        listPane = { AnimatedPane { list() } },
        detailPane = {
            AnimatedPane {
                if (current == null) {
                    // Beside the list, the empty pane says what goes there; on a phone it's only sliding out.
                    if (directive.maxHorizontalPartitions > 1) Placeholder(placeholder)
                } else {
                    // Each item gets its own state, so the next one doesn't open at the last one's tab or scroll.
                    key(details.items.size, current) { detail(current, isAlone) }
                }
            }
        },
        paneExpansionDragHandle = { state -> PaneHandle(state) },
    )
}

/** How a page lays out its panes: two side by side from 600 dp, where the Material default waits for 840, and one below. */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
private fun paneDirective(): PaneScaffoldDirective = calculatePaneScaffoldDirectiveWithTwoPanesOnMediumWidth(currentWindowAdaptiveInfo())

/** Whether the window is wide enough, from 600 dp, for a page's list and its open item side by side. */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
fun showsTwoPanes(): Boolean = paneDirective().maxHorizontalPartitions > 1

/** The handle between the panes: drag it to resize them. */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
private fun ThreePaneScaffoldScope.PaneHandle(state: PaneExpansionState) {
    val interactions = remember { MutableInteractionSource() }
    Box(
        Modifier.paneExpansionDraggable(state, 48.dp, interactions, state.defaultDragHandleSemantics()),
        contentAlignment = Alignment.Center,
    ) {
        Box(Modifier.size(width = 4.dp, height = 48.dp).background(MaterialTheme.colorScheme.outline, CircleShape))
    }
}

@Composable
private fun Placeholder(text: String) {
    Box(Modifier.fillMaxSize().padding(Spacing.xl), contentAlignment = Alignment.Center) {
        Text(text, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    }
}
