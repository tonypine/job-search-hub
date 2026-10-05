package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/** Something a view lets you do: its word, an optional symbol, and what it does. */
data class HubAction(val label: String, val icon: ImageVector? = null, val enabled: Boolean = true, val onClick: () -> Unit)

/**
 * How far the snackbar keeps up from the bottom so it doesn't cover a docked
 * [ActionBar]: the tallest bar showing, or nothing on a page without one.
 * Two bars show at once only while one page gives way to the next.
 */
@Stable
class SnackbarClearance {
    private val bars = mutableStateMapOf<Any, Dp>()

    val height: Dp get() = bars.values.maxOrNull() ?: 0.dp

    /** Called on every placement, so it writes only a new height. */
    fun show(bar: Any, height: Dp) {
        if (bars[bar] != height) bars[bar] = height
    }

    fun hide(bar: Any) {
        bars.remove(bar)
    }
}

/**
 * A view's actions as a medium button group docked at the bottom, in thumb
 * reach: the one [primary] filled, a [tonal] and an [outlined] secondary. The
 * rest go in the top app bar's [OverflowMenu]. While it shows, it reports
 * its height to [clearance], so the snackbar sits above it rather than over
 * its buttons.
 */
@Composable
fun ActionBar(
    primary: HubAction,
    modifier: Modifier = Modifier,
    tonal: HubAction? = null,
    outlined: HubAction? = null,
    clearance: SnackbarClearance? = null,
) {
    val density = LocalDensity.current
    val bar = remember { Any() }
    DisposableEffect(clearance) { onDispose { clearance?.hide(bar) } }
    Surface(
        // On every placement rather than on a size change: the next job's bar can reuse this one's layout at the same size.
        modifier.fillMaxWidth().onGloballyPositioned { clearance?.show(bar, with(density) { it.size.height.toDp() }) },
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp),
    ) {
        Row(Modifier.padding(horizontal = Spacing.l, vertical = Spacing.m), horizontalArrangement = Arrangement.spacedBy(Spacing.s)) {
            val size = Modifier.heightIn(min = 56.dp)
            outlined?.let {
                OutlinedButton(it.onClick, size.weight(1f), enabled = it.enabled) { ActionLabel(it) }
            }
            tonal?.let {
                FilledTonalButton(it.onClick, size.weight(1f), enabled = it.enabled) { ActionLabel(it) }
            }
            Button(primary.onClick, size.weight(1.8f), enabled = primary.enabled) { ActionLabel(primary) }
        }
    }
}

@Composable
private fun ActionLabel(action: HubAction) {
    action.icon?.let { Icon(it, contentDescription = null, modifier = Modifier.padding(end = Spacing.s).size(24.dp)) }
    Text(action.label, style = MaterialTheme.typography.titleMedium, maxLines = 1)
}

/** The `⋮` icon button and its menu, each item with a leading symbol. */
@Composable
fun OverflowMenu(actions: List<HubAction>, modifier: Modifier = Modifier) {
    var isOpen by remember { mutableStateOf(false) }
    Box(modifier) {
        IconButton(onClick = { isOpen = true }) { Icon(Icons.Rounded.MoreVert, contentDescription = "More") }
        DropdownMenu(expanded = isOpen, onDismissRequest = { isOpen = false }) {
            actions.forEach { action ->
                DropdownMenuItem(
                    text = { Text(action.label) },
                    leadingIcon = action.icon?.let { icon -> { Icon(icon, contentDescription = null) } },
                    enabled = action.enabled,
                    onClick = {
                        isOpen = false
                        action.onClick()
                    },
                )
            }
        }
    }
}
