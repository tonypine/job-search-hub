package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.FlowRowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AssistChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

/** The entity a header links up to, such as a job's company. */
data class ParentLink(val name: String, val onClick: () -> Unit)

/**
 * An entity's header: its name, the parent as an assist chip that opens it,
 * one line of facts, and up to three [chips]. With no [title], the name is in
 * the app bar above.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun EntityHeader(
    title: String?,
    modifier: Modifier = Modifier,
    parent: ParentLink? = null,
    facts: String? = null,
    chips: (@Composable FlowRowScope.() -> Unit)? = null,
) {
    Column(modifier.fillMaxWidth().padding(horizontal = Spacing.l), verticalArrangement = Arrangement.spacedBy(Spacing.s)) {
        title?.let { Text(it, style = MaterialTheme.typography.headlineMedium, modifier = Modifier.semantics { heading() }) }
        if (parent != null || !facts.isNullOrBlank()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(Spacing.s), itemVerticalAlignment = Alignment.CenterVertically) {
                parent?.let {
                    AssistChip(
                        onClick = it.onClick, label = { Text(it.name) },
                        leadingIcon = { Monogram(it.name, size = 24.dp) },
                    )
                }
                facts?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        chips?.let {
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalArrangement = Arrangement.spacedBy(Spacing.s),
                content = it,
            )
        }
    }
}
