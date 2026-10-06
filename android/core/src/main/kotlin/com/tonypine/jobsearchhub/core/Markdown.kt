package com.tonypine.jobsearchhub.core

/** A run of a line's text, bold or not. */
data class TextRun(val text: String, val bold: Boolean = false)

/**
 * A Markdown document as the blocks the job screen draws, the same as the
 * Mac's `MarkdownBlocks`: headings, bullets and paragraphs, each as runs of
 * plain and bold text.
 */
sealed interface MarkdownBlock {
    val runs: List<TextRun>

    data class Heading(val level: Int, override val runs: List<TextRun>) : MarkdownBlock

    data class Bullet(override val runs: List<TextRun>) : MarkdownBlock

    data class Paragraph(override val runs: List<TextRun>) : MarkdownBlock
}

object Markdown {
    /** Consecutive plain lines form one paragraph, keeping their line breaks; a blank line ends it. */
    fun parse(markdown: String): List<MarkdownBlock> {
        val blocks = mutableListOf<MarkdownBlock>()
        val paragraphLines = mutableListOf<String>()
        fun closeParagraph() {
            if (paragraphLines.isNotEmpty()) {
                blocks += MarkdownBlock.Paragraph(parseInline(paragraphLines.joinToString("\n")))
                paragraphLines.clear()
            }
        }
        for (rawLine in markdown.lines()) {
            val line = rawLine.trim()
            val headingLevel = line.takeWhile { it == '#' }.length
            when {
                line.isEmpty() -> closeParagraph()
                headingLevel in 1..6 && line.getOrNull(headingLevel) == ' ' -> {
                    closeParagraph()
                    blocks += MarkdownBlock.Heading(headingLevel, parseInline(line.drop(headingLevel + 1)))
                }
                line.startsWith("- ") || line.startsWith("* ") -> {
                    closeParagraph()
                    blocks += MarkdownBlock.Bullet(parseInline(line.drop(2)))
                }
                else -> paragraphLines += line
            }
        }
        closeParagraph()
        return blocks
    }

    /** Text between a pair of `**` is bold; a `**` without its pair stays as written. */
    fun parseInline(text: String): List<TextRun> {
        val parts = text.split("**").toMutableList()
        if (parts.size % 2 == 0) {
            val unpaired = parts.removeAt(parts.lastIndex)
            parts[parts.lastIndex] += "**$unpaired"
        }
        return parts.mapIndexedNotNull { index, part -> part.takeIf { it.isNotEmpty() }?.let { TextRun(it, bold = index % 2 == 1) } }
    }
}
