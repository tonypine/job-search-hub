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
import com.tonypine.jobsearchhub.data.HubFailure

/** The advice for a call that failed on the way to the hub. */
private const val NETWORK_ADVICE = "Check that the Mac is on and the phone can reach it, then try again."

/**
 * A failed call to the hub under the screen's [title], or, when the hub no longer serves this version of the app,
 * under an update headline with the hub's words as the advice.
 */
@Composable
fun HubErrorView(title: String, failure: HubFailure, modifier: Modifier = Modifier, onRetry: (() -> Unit)? = null) {
    HubErrorView(failure.title(title), failure.details, modifier, advice = failure.advice ?: NETWORK_ADVICE, onRetry = onRetry)
}

/**
 * What failed and what to do about it, a retry when there is one, and the raw
 * error behind Details for when the advice isn't enough.
 */
@Composable
fun HubErrorView(
    title: String,
    error: String?,
    modifier: Modifier = Modifier,
    advice: String = NETWORK_ADVICE,
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
