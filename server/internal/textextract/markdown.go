package textextract

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	markupTag          = regexp.MustCompile(`<[^>]*>`)
	markupTagName      = regexp.MustCompile(`^<\s*(/?)\s*([a-zA-Z][a-zA-Z0-9]*)`)
	markdownLineMarker = regexp.MustCompile(`(?m)^[ \t]*(?:(?:#{1,6}|[-*+]|[0-9]+\.)[ \t]+)+`)
)

// ConvertHTMLToMarkdown keeps a posting's structure as Markdown: headings
// become "### " lines, list items "- " or "1. " lines, indented when nested,
// bold "**…**", and paragraphs and line breaks lines. Links keep their text.
// Scripts, styles and other tags are dropped and entities decoded, as in
// ConvertHTMLToText.
func ConvertHTMLToMarkdown(markup string) string {
	markup = hiddenBlocks.ReplaceAllString(markup, "")
	var writer markdownWriter
	last := 0
	for _, span := range markupTag.FindAllStringIndex(markup, -1) {
		writer.writeText(html.UnescapeString(markup[last:span[0]]))
		if name := markupTagName.FindStringSubmatch(markup[span[0]:span[1]]); name != nil {
			writer.writeTag(strings.ToLower(name[2]), name[1] == "/")
		}
		last = span[1]
	}
	writer.writeText(html.UnescapeString(markup[last:]))
	return writer.finish()
}

// ConvertMarkdownToText drops the Markdown a posting's text carries, so a
// quote from it reads as plain words: heading and list markers at the start
// of a line, and bold markers.
func ConvertMarkdownToText(markdown string) string {
	return strings.ReplaceAll(markdownLineMarker.ReplaceAllString(markdown, ""), "**", "")
}

type markdownList struct {
	ordered bool
	items   int
}

// markdownWriter writes Markdown as tags and text arrive. Breaks and spaces
// wait for the next word, so none trails a line or the text, and a heading
// or list marker is written only before a line's first word.
type markdownWriter struct {
	out          strings.Builder
	linePrefix   string
	lineHasText  bool
	pendingSpace bool
	pendingBreak int
	headings     int
	lists        []markdownList
	bolds        int
	isBoldOpen   bool
}

func (writer *markdownWriter) writeTag(name string, isClosing bool) {
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		writer.breakLine(2)
		if isClosing {
			writer.headings = max(writer.headings-1, 0)
		} else {
			writer.headings++
			writer.linePrefix = "### "
		}
	case "ul", "ol":
		if isClosing && len(writer.lists) > 0 {
			writer.lists = writer.lists[:len(writer.lists)-1]
		}
		// A list is a paragraph of its own; a nested one is lines in its item.
		if len(writer.lists) == 0 {
			writer.breakLine(2)
		} else {
			writer.breakLine(1)
		}
		if !isClosing {
			writer.lists = append(writer.lists, markdownList{ordered: name == "ol"})
		}
	case "li":
		writer.breakLine(1)
		if !isClosing {
			writer.linePrefix = writer.startItem()
		}
	case "b", "strong":
		// The closing marker waits for the next word, so bold that ends
		// where more starts stays one run.
		if !isClosing {
			writer.bolds++
		} else {
			writer.bolds = max(writer.bolds-1, 0)
		}
	case "p", "div", "section", "article", "header", "footer", "blockquote", "table", "hr":
		writer.breakBlock(2)
	case "br", "tr":
		writer.breakBlock(1)
	case "td", "th":
		writer.space()
	}
}

// startItem counts an item in the innermost list and returns its marker.
func (writer *markdownWriter) startItem() string {
	if len(writer.lists) == 0 {
		return "- "
	}
	list := &writer.lists[len(writer.lists)-1]
	list.items++
	indent := strings.Repeat("  ", len(writer.lists)-1)
	if list.ordered {
		return fmt.Sprintf("%s%d. ", indent, list.items)
	}
	return indent + "- "
}

// isInline says text stays on its line: a heading or a list item is one line.
func (writer *markdownWriter) isInline() bool {
	return writer.headings > 0 || len(writer.lists) > 0
}

// breakBlock ends a paragraph or a line, unless that would split a heading
// or a list item, where it is a space.
func (writer *markdownWriter) breakBlock(lines int) {
	if writer.isInline() {
		writer.space()
		return
	}
	writer.breakLine(lines)
}

// breakLine starts a new line, or a new paragraph after a blank one when
// lines is 2, before the next word. Bold ends with its line.
func (writer *markdownWriter) breakLine(lines int) {
	writer.closeBold()
	writer.pendingBreak = max(writer.pendingBreak, lines)
	writer.pendingSpace = false
	writer.linePrefix = ""
}

func (writer *markdownWriter) closeBold() {
	if writer.isBoldOpen {
		writer.out.WriteString("**")
		writer.isBoldOpen = false
	}
}

func (writer *markdownWriter) space() {
	if writer.lineHasText {
		writer.pendingSpace = true
	}
}

// writeText writes text's words, a line break where it breaks a line outside
// a heading or list, and a space between words.
func (writer *markdownWriter) writeText(text string) {
	for index, line := range strings.Split(text, "\n") {
		if index > 0 {
			writer.breakBlock(1)
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			if line != "" {
				writer.space()
			}
			continue
		}
		if first, _ := utf8.DecodeRuneInString(line); unicode.IsSpace(first) {
			writer.space()
		}
		for wordIndex, word := range words {
			if wordIndex > 0 {
				writer.space()
			}
			writer.writeWord(word)
		}
		if last, _ := utf8.DecodeLastRuneInString(line); unicode.IsSpace(last) {
			writer.space()
		}
	}
}

func (writer *markdownWriter) writeWord(word string) {
	if writer.bolds == 0 {
		writer.closeBold()
	}
	if writer.pendingBreak > 0 && writer.out.Len() > 0 {
		writer.out.WriteString(strings.Repeat("\n", writer.pendingBreak))
		writer.lineHasText = false
	}
	writer.pendingBreak = 0
	if !writer.lineHasText {
		writer.out.WriteString(writer.linePrefix)
		writer.linePrefix = ""
		writer.lineHasText = true
	} else if writer.pendingSpace {
		writer.out.WriteByte(' ')
	}
	writer.pendingSpace = false
	if writer.bolds > 0 && writer.headings == 0 && !writer.isBoldOpen {
		writer.out.WriteString("**")
		writer.isBoldOpen = true
	}
	writer.out.WriteString(word)
}

func (writer *markdownWriter) finish() string {
	writer.closeBold()
	return writer.out.String()
}
