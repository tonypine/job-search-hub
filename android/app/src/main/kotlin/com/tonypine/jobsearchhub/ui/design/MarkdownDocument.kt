package com.tonypine.jobsearchhub.ui.design

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import com.tonypine.jobsearchhub.core.Markdown
import com.tonypine.jobsearchhub.core.MarkdownBlock
import com.tonypine.jobsearchhub.core.TextRun

/**
 * Markdown drawn as a document, as on the Mac: headings in bold at a larger
 * size, bullets and paragraphs, with bold inside them. A job's posting uses it.
 */
@Composable
fun MarkdownDocument(markdown: String, modifier: Modifier = Modifier) {
    val blocks = remember(markdown) { Markdown.parse(markdown) }
    val body = MaterialTheme.typography.bodyMedium
    Column(modifier, verticalArrangement = Arrangement.spacedBy(Spacing.s)) {
        blocks.forEach { block ->
            when (block) {
                is MarkdownBlock.Heading -> Text(
                    block.runs.toAnnotatedString(),
                    style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                    modifier = Modifier.padding(top = Spacing.xs).semantics { heading() },
                )
                is MarkdownBlock.Bullet -> Row(horizontalArrangement = Arrangement.spacedBy(Spacing.s)) {
                    Text("•", style = body)
                    Text(block.runs.toAnnotatedString(), style = body, modifier = Modifier.weight(1f))
                }
                is MarkdownBlock.Paragraph -> Text(block.runs.toAnnotatedString(), style = body)
            }
        }
    }
}

private fun List<TextRun>.toAnnotatedString(): AnnotatedString = buildAnnotatedString {
    for (run in this@toAnnotatedString) {
        if (run.bold) withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { append(run.text) } else append(run.text)
    }
}
