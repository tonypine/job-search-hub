package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Help
import androidx.compose.material.icons.rounded.Cancel
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.Info
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.core.Screen
import com.tonypine.jobsearchhub.core.Tone

/** A status word as a Material 3 label: the tone's container, 8 dp corners, and an optional symbol. */
@Composable
fun ToneChip(text: String, tone: Tone, modifier: Modifier = Modifier, icon: ImageVector? = null) {
    val colors = HubTheme.colors.of(tone)
    Row(
        modifier
            .heightIn(min = 24.dp)
            .background(colors.container, MaterialTheme.shapes.small)
            .padding(start = if (icon == null) Spacing.s else 6.dp, end = Spacing.s),
        horizontalArrangement = Arrangement.spacedBy(Spacing.xs),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        icon?.let { Icon(it, contentDescription = null, tint = colors.onContainer, modifier = Modifier.size(16.dp)) }
        Text(text, style = MaterialTheme.typography.labelMedium, color = colors.onContainer, maxLines = 1)
    }
}

/** A screen verdict's symbol; null is information no rule judges. */
val Screen?.symbol: ImageVector
    get() = when (this) {
        Screen.PASSES -> Icons.Rounded.CheckCircle
        Screen.UNCLEAR -> Icons.AutoMirrored.Rounded.Help
        Screen.FAILS -> Icons.Rounded.Cancel
        null -> Icons.Rounded.Info
    }

/** A company's or person's first letter in a tonal circle, the leading element of a row. */
@Composable
fun Monogram(name: String, modifier: Modifier = Modifier, tone: Tone = Tone.ACCENT, size: Dp = 40.dp) {
    val colors = HubTheme.colors.of(tone)
    Box(modifier.size(size).background(colors.container, CircleShape), contentAlignment = Alignment.Center) {
        Text(
            name.firstOrNull { it.isLetterOrDigit() }?.uppercase() ?: "?",
            style = if (size < 32.dp) MaterialTheme.typography.labelMedium else MaterialTheme.typography.titleMedium,
            color = colors.onContainer,
        )
    }
}
