package com.tonypine.jobsearchhub.ui

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext

@Composable
fun HubTheme(content: @Composable () -> Unit) {
    val context = LocalContext.current
    val dark = isSystemInDarkTheme()
    // Dynamic colour arrived in Android 12; Android 11 gets the plain palette.
    val colors = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> plainDark
        else -> plainLight
    }
    MaterialTheme(colorScheme = colors, content = content)
}

/** The plain palette: previews, and phones without dynamic colour. */
internal val plainLight = lightColorScheme()
internal val plainDark = darkColorScheme()
