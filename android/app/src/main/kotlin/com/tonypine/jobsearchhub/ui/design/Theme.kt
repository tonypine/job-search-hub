package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.tonypine.jobsearchhub.core.Tone

/**
 * The hub's look: the Material 3 scheme built from Hub Indigo, the tones as
 * custom colors with container roles, the type scale and the shape scale.
 * The scheme is the fidelity variant, which keeps the seed, #4B49D6, as primary.
 */
@Composable
fun HubTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val scheme = if (dark) hubDark else hubLight
    CompositionLocalProvider(LocalHubColors provides hubColors(scheme, dark)) {
        MaterialTheme(colorScheme = scheme, typography = hubTypography, shapes = hubShapes, content = content)
    }
}

object HubTheme {
    /** The tones and the surface a list item or card sits on. */
    val colors: HubColors
        @Composable @ReadOnlyComposable get() = LocalHubColors.current
}

/** A tone's color for icons, its container for chips and the text on that container. */
@Immutable
data class ToneColors(val color: Color, val container: Color, val onContainer: Color)

@Immutable
data class HubColors(private val tones: Map<Tone, ToneColors>, val item: Color) {
    fun of(tone: Tone): ToneColors = tones.getValue(tone)
}

private val LocalHubColors = staticCompositionLocalOf { hubColors(hubLight, dark = false) }

/** The four tones, harmonized toward the seed; accent is the scheme's primary and negative its error. */
private fun hubColors(scheme: ColorScheme, dark: Boolean) = HubColors(
    tones = mapOf(
        Tone.ACCENT to ToneColors(scheme.primary, scheme.primaryContainer, scheme.onPrimaryContainer),
        Tone.POSITIVE to if (dark) {
            ToneColors(Color(0xFF8DD7A3), Color(0xFF00522D), Color(0xFFB4F1C6))
        } else {
            ToneColors(Color(0xFF1B6D3F), Color(0xFFB4F1C6), Color(0xFF00210E))
        },
        Tone.CAUTION to if (dark) {
            ToneColors(Color(0xFFFFB86E), Color(0xFF693C00), Color(0xFFFFDCBE))
        } else {
            ToneColors(Color(0xFF8A5100), Color(0xFFFFDCBE), Color(0xFF2C1600))
        },
        Tone.NEGATIVE to ToneColors(scheme.error, scheme.errorContainer, scheme.onErrorContainer),
        Tone.NEUTRAL to ToneColors(scheme.onSurfaceVariant, scheme.surfaceContainerHighest, scheme.onSurfaceVariant),
    ),
    item = if (dark) scheme.surfaceContainerHigh else scheme.surfaceContainerLowest,
)

/** The page is the surface; list items and cards sit on it in `HubColors.item`. */
private val hubLight = lightColorScheme(
    primary = Color(0xFF4B49D6),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFE2DFFF),
    onPrimaryContainer = Color(0xFF100069),
    inversePrimary = Color(0xFFC3C0FF),
    secondary = Color(0xFF5D5C72),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFE3E0F9),
    onSecondaryContainer = Color(0xFF1A1A2C),
    tertiary = Color(0xFF7D5260),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFFFD8E4),
    onTertiaryContainer = Color(0xFF31111D),
    background = Color(0xFFF0ECF6),
    onBackground = Color(0xFF1B1B21),
    surface = Color(0xFFF0ECF6),
    onSurface = Color(0xFF1B1B21),
    surfaceVariant = Color(0xFFE4E1EC),
    onSurfaceVariant = Color(0xFF47464F),
    surfaceTint = Color(0xFF4B49D6),
    inverseSurface = Color(0xFF303036),
    inverseOnSurface = Color(0xFFF3EFF7),
    error = Color(0xFFBA1A1A),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFFDAD6),
    onErrorContainer = Color(0xFF410002),
    outline = Color(0xFF787680),
    outlineVariant = Color(0xFFC8C5D0),
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFFFCF8FF),
    surfaceDim = Color(0xFFDCD9E0),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFF6F2FC),
    surfaceContainer = Color(0xFFF0ECF6),
    surfaceContainerHigh = Color(0xFFEAE7F1),
    surfaceContainerHighest = Color(0xFFE4E1EB),
)

private val hubDark = darkColorScheme(
    primary = Color(0xFFC3C0FF),
    onPrimary = Color(0xFF1F1A8C),
    primaryContainer = Color(0xFF3533BE),
    onPrimaryContainer = Color(0xFFE2DFFF),
    inversePrimary = Color(0xFF4B49D6),
    secondary = Color(0xFFC6C4DD),
    onSecondary = Color(0xFF2F2F42),
    secondaryContainer = Color(0xFF464559),
    onSecondaryContainer = Color(0xFFE3E0F9),
    tertiary = Color(0xFFEFB8C8),
    onTertiary = Color(0xFF492532),
    tertiaryContainer = Color(0xFF633B48),
    onTertiaryContainer = Color(0xFFFFD8E4),
    background = Color(0xFF131318),
    onBackground = Color(0xFFE4E1E9),
    surface = Color(0xFF131318),
    onSurface = Color(0xFFE4E1E9),
    surfaceVariant = Color(0xFF47464F),
    onSurfaceVariant = Color(0xFFC8C5D0),
    surfaceTint = Color(0xFFC3C0FF),
    inverseSurface = Color(0xFFE4E1E9),
    inverseOnSurface = Color(0xFF303036),
    error = Color(0xFFFFB4AB),
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = Color(0xFFFFDAD6),
    outline = Color(0xFF928F9A),
    outlineVariant = Color(0xFF47464F),
    scrim = Color(0xFF000000),
    surfaceBright = Color(0xFF39383F),
    surfaceDim = Color(0xFF131318),
    surfaceContainerLowest = Color(0xFF0E0E13),
    surfaceContainerLow = Color(0xFF1B1B21),
    surfaceContainer = Color(0xFF1F1F25),
    surfaceContainerHigh = Color(0xFF2A292F),
    surfaceContainerHighest = Color(0xFF35343A),
)

/**
 * The Material 3 type scale in Roboto, in `sp` so it follows the font size
 * setting. The roles the hub uses: `headlineMedium` for an entity's name,
 * `titleLarge` for a page, `titleSmall` for a section, `bodyMedium` for text you
 * read, `bodySmall` for secondary text and `labelSmall` for captions.
 */
private val hubTypography = Typography()

/** The Material 3 shape scale: 4 fields, 8 chips and labels, 12 menus, 16 cards, 28 dialogs and sheets. */
private val hubShapes = Shapes(
    extraSmall = RoundedCornerShape(4.dp),
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(16.dp),
    extraLarge = RoundedCornerShape(28.dp),
)

/** The spacing scale: rows within a section are `s` apart, sections `l`, and pages pad by `l` on a phone. */
object Spacing {
    val xs = 4.dp
    val s = 8.dp
    val m = 12.dp
    val l = 16.dp
    val xl = 24.dp
    val xxl = 32.dp
}
