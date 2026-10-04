package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import com.tonypine.jobsearchhub.core.Tone

/**
 * What failed and what to do about it, a retry when there is one, and the raw
 * error behind Details for when the advice isn't enough.
 */
@Composable
fun HubErrorView(
    title: String,
    error: String?,
    modifier: Modifier = Modifier,
    advice: String = "Check that the Mac is on and the phone can reach it, then try again.",
    onRetry: (() -> Unit)? = null,
) {
    var showsDetails by rememberSaveable { mutableStateOf(false) }
    Column(modifier.fillMaxWidth().padding(Spacing.l), verticalArrangement = Arrangement.spacedBy(Spacing.s)) {
        Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.ErrorOutline, contentDescription = null, tint = HubTheme.colors.of(Tone.NEGATIVE).color)
            Text(title, style = MaterialTheme.typography.titleSmall)
        }
        Text(advice, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
            onRetry?.let { FilledTonalButton(onClick = it) { Text("Try again") } }
            if (!error.isNullOrBlank()) {
                TextButton(onClick = { showsDetails = !showsDetails }) { Text(if (showsDetails) "Hide details" else "Details") }
            }
        }
        if (showsDetails && !error.isNullOrBlank()) {
            SelectionContainer {
                Text(error, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
