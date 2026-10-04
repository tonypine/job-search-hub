package com.tonypine.jobsearchhub.ui

import android.text.format.DateUtils
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.core.PipelinePhase
import com.tonypine.jobsearchhub.ui.design.Spacing
import java.time.Duration
import java.time.Instant

/**
 * Records a follow-up on a card, with an optional note for the change log.
 * The hub restarts the phase's count from now, so the sheet says when the
 * next one falls due rather than asking.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FollowedUpSheet(card: PipelineCard, phase: PipelinePhase?, onDismiss: () -> Unit, onSave: (note: String) -> Unit) {
    val context = LocalContext.current
    var note by rememberSaveable(card.id) { mutableStateOf("") }
    val now = Instant.now()
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
        Column(Modifier.fillMaxWidth().padding(horizontal = Spacing.xl).padding(bottom = Spacing.xl), verticalArrangement = Arrangement.spacedBy(Spacing.m)) {
            Text("Followed up with ${card.companyName ?: card.title}", style = MaterialTheme.typography.headlineSmall)
            val days = card.daysInPhase(now)
            Text(
                listOfNotNull(card.jobTitle.takeIf { card.companyName != null }, phase?.let { "${dayCount(days)} in ${it.name}" }).joinToString(" · "),
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text("Next follow-up", style = MaterialTheme.typography.titleSmall)
            val interval = phase?.followUpDays
            Text(
                if (phase == null || interval == null) {
                    "${phase?.name ?: "This phase"} asks for no follow-ups."
                } else {
                    val next = DateUtils.formatDateTime(
                        context, now.plus(Duration.ofDays(interval.toLong())).toEpochMilli(),
                        DateUtils.FORMAT_SHOW_DATE or DateUtils.FORMAT_SHOW_WEEKDAY or DateUtils.FORMAT_ABBREV_ALL,
                    )
                    "In ${dayCount(interval)}, $next, as ${phase.name} asks."
                },
                style = MaterialTheme.typography.bodyMedium,
            )
            OutlinedTextField(note, { note = it }, label = { Text("Note") }, modifier = Modifier.fillMaxWidth())
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(Spacing.s, Alignment.End)) {
                TextButton(onClick = onDismiss) { Text("Cancel") }
                Button(onClick = { onSave(note) }) { Text("Save") }
            }
        }
    }
}
