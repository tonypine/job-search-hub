package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.core.Tone

/** How someone can get the owner in: a contact, a connection, an introducer or a recruiter. */
enum class Relation(val word: String) {
    CONTACT("Contact"),
    CONNECTION("Connection"),
    INTRODUCER("Introducer"),
    RECRUITER("Recruiter"),
}

/** A person: their name, relation, role or note, and the action that fits them, if any. */
@Composable
fun PersonRow(name: String, relation: Relation, modifier: Modifier = Modifier, role: String? = null, action: HubAction? = null) {
    Row(
        modifier.fillMaxWidth().heightIn(min = 72.dp).padding(horizontal = Spacing.l, vertical = Spacing.m),
        horizontalArrangement = Arrangement.spacedBy(Spacing.l),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Monogram(name, tone = Tone.NEUTRAL)
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(Spacing.xs)) {
            Text(name, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s), verticalAlignment = Alignment.CenterVertically) {
                ToneChip(relation.word, Tone.NEUTRAL)
                role?.takeIf { it.isNotBlank() }?.let {
                    Text(
                        it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 2, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
        action?.let { FilledTonalButton(onClick = it.onClick, enabled = it.enabled) { Text(it.label) } }
    }
}
