package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.assertEquals

/** A posting as the hub keeps it, from a board's HTML. */
class MarkdownTest {
    @Test
    fun aPostingBecomesHeadingsBulletsAndParagraphs() {
        val blocks = Markdown.parse(
            """
            ### What you'll do

            Build the **checkout** app.
            Remote.

            - Ship **React** features
              - Weekly
            * Talk to customers

            1. Apply
            #not a heading
            """.trimIndent(),
        )

        assertEquals(
            listOf(
                MarkdownBlock.Heading(3, listOf(TextRun("What you'll do"))),
                MarkdownBlock.Paragraph(listOf(TextRun("Build the "), TextRun("checkout", bold = true), TextRun(" app.\nRemote."))),
                MarkdownBlock.Bullet(listOf(TextRun("Ship "), TextRun("React", bold = true), TextRun(" features"))),
                MarkdownBlock.Bullet(listOf(TextRun("Weekly"))),
                MarkdownBlock.Bullet(listOf(TextRun("Talk to customers"))),
                MarkdownBlock.Paragraph(listOf(TextRun("1. Apply\n#not a heading"))),
            ),
            blocks,
        )
    }

    @Test
    fun boldNeedsItsPair() {
        assertEquals(listOf(TextRun("Pay:", bold = true), TextRun(" $100k")), Markdown.parseInline("**Pay:** $100k"))
        assertEquals(listOf(TextRun("5 **stars")), Markdown.parseInline("5 **stars"))
        assertEquals(listOf(TextRun("a "), TextRun("b", bold = true), TextRun(" c **d")), Markdown.parseInline("a **b** c **d"))
        assertEquals(emptyList(), Markdown.parseInline(""))
    }
}
