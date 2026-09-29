// Package textextract reads the text of the files the owner uploads: PDFs,
// HTML pages and plain text.
package textextract

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// Extract returns the text of a file of contentType named name. A file whose
// text can't be read returns an error that says why.
func Extract(contentType, name string, content []byte) (string, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	extension := strings.ToLower(filepath.Ext(name))
	var text string
	var err error
	switch {
	case mediaType == "application/pdf" || extension == ".pdf" || bytes.HasPrefix(content, []byte("%PDF-")):
		text, err = readPDF(content)
	case mediaType == "text/html" || extension == ".html" || extension == ".htm":
		text = ConvertHTMLToText(decodeUTF8(content))
	case strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" ||
		extension == ".txt" || extension == ".md" || extension == ".csv" || extension == ".json":
		text = strings.TrimSpace(decodeUTF8(content))
	default:
		return "", fmt.Errorf("can't read text from a %s file", describeType(mediaType, extension))
	}
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", errors.New("the file has no text to read; a scanned PDF needs text recognition first")
	}
	return text, nil
}

func readPDF(content []byte) (text string, err error) {
	// The PDF reader panics on some malformed files.
	defer func() {
		if recovered := recover(); recovered != nil {
			text, err = "", fmt.Errorf("the PDF can't be read: %v", recovered)
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("the PDF can't be read: %w", err)
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("the PDF's text can't be read: %w", err)
	}
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(plain); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func decodeUTF8(content []byte) string {
	if utf8.Valid(content) {
		return string(content)
	}
	return strings.ToValidUTF8(string(content), "")
}

func describeType(mediaType, extension string) string {
	if mediaType != "" {
		return mediaType
	}
	if extension != "" {
		return extension
	}
	return "binary"
}

var (
	lineBreakTags = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|tr|h[1-6])>`)
	hiddenBlocks  = regexp.MustCompile(`(?is)<(style|script|head)[^>]*>.*?</(style|script|head)>`)
	anyMarkup     = regexp.MustCompile(`<[^>]*>`)
	blankRuns     = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

// ConvertHTMLToText drops a page's markup, scripts and styles, keeping its
// text with a line break per paragraph, list item or table row.
func ConvertHTMLToText(markup string) string {
	text := hiddenBlocks.ReplaceAllString(markup, "")
	text = lineBreakTags.ReplaceAllString(text, "\n")
	text = html.UnescapeString(anyMarkup.ReplaceAllString(text, ""))
	return strings.TrimSpace(blankRuns.ReplaceAllString(text, "\n\n"))
}
