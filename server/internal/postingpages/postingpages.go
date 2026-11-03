// Package postingpages reads the posting behind any link a job is added
// from, most often a company's own careers page. A page that links to the
// posting on Greenhouse, Lever or Ashby names it, so the provider's API is
// read instead; one that carries schema.org JobPosting data gives that; any
// other gives its main content as the posting's text.
package postingpages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

const (
	// maximumPageBytes bounds the page read; a careers page is far smaller.
	maximumPageBytes = 5 << 20
	// MinimumTextLength is the shortest main content taken for a posting's
	// text, in characters: less is a page that draws its posting with
	// JavaScript, which leaves only a line asking to turn it on.
	MinimumTextLength = 200
	userAgent         = "Mozilla/5.0 (compatible; JobSearchHub/1.0; +https://github.com/tonypine/job-search-hub)"
)

// How a page's posting was read.
const (
	// ReadFromStructuredData is a schema.org JobPosting the page carries.
	ReadFromStructuredData = "structured_data"
	// ReadFromPageText is the page's main content.
	ReadFromPageText = "page_text"
)

// Page is what a posting's page gives. Reference names the posting on a
// provider when the page links to it, to be read there. Posting holds what
// the page itself says, read as ReadFrom says, for when there is no
// Reference or the provider can't be read; its Description is empty when
// the page has no text to read.
type Page struct {
	Reference *jobboards.PostingReference
	Posting   store.JobPosting
	ReadFrom  string
}

// Reader fetches pages over HTTP.
type Reader struct {
	HTTPClient *http.Client
}

func NewReader() *Reader {
	return &Reader{HTTPClient: &http.Client{Timeout: 20 * time.Second}}
}

// ReadPage fetches the page at pageURL and reads its posting. An error means
// the page couldn't be fetched.
func (reader *Reader) ReadPage(ctx context.Context, pageURL string) (Page, error) {
	parsed, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return Page{}, errors.New("the link isn't an http(s) URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Page{}, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	response, err := reader.HTTPClient.Do(request)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("the site answered %s", response.Status)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" && !strings.Contains(contentType, "html") {
		return Page{}, fmt.Errorf("the link is a %s, not a web page", strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumPageBytes))
	if err != nil {
		return Page{}, err
	}
	return ParsePage(response.Request.URL, body), nil
}

// ParsePage reads the posting in a page fetched from pageURL: the provider
// posting it links to, else its JobPosting data, else its main content.
func ParsePage(pageURL *url.URL, body []byte) Page {
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Page{}
	}
	var page Page
	if reference, found := findPostingReference(pageURL, document); found {
		page.Reference = &reference
	}
	if posting, found := readJobPostingData(document); found {
		posting.URL = pageURL.String()
		page.Posting, page.ReadFrom = posting, ReadFromStructuredData
		return page
	}
	page.Posting, page.ReadFrom = store.JobPosting{URL: pageURL.String(), Title: readTitle(document)}, ReadFromPageText
	if text := readMainText(document); utf8.RuneCountInString(text) >= MinimumTextLength {
		page.Posting.Description = text
	}
	return page
}

// pageURLPattern finds the URLs a page names, in its links and embeds and in
// its scripts alike.
var pageURLPattern = regexp.MustCompile(`https?:(?:\\?/){2}(?:job-boards\.greenhouse\.io|boards\.greenhouse\.io|jobs\.lever\.co|jobs\.ashbyhq\.com)[^\s"'<>\\)]*`)

// findPostingReference finds the provider posting the page links to, as its
// apply link or its embedded application does. A page that links to several
// keeps the one whose id its own URL holds, and names none when no URL
// does: a list of jobs isn't one job's page. A provider's board embedded in
// the page names its posting through the page URL's gh_jid or ashby_jid.
func findPostingReference(pageURL *url.URL, document *html.Node) (jobboards.PostingReference, bool) {
	var postings []jobboards.PostingReference
	boards := map[string]string{}
	visit := func(raw string) {
		raw = strings.ReplaceAll(html.UnescapeString(raw), `\/`, "/")
		if reference, ok := jobboards.ParsePostingURL(raw); ok {
			for _, known := range postings {
				if known == reference {
					return
				}
			}
			postings = append(postings, reference)
			return
		}
		if provider, boardToken, ok := jobboards.ParseBoardURL(raw); ok {
			boards[provider] = boardToken
		}
	}
	walk(document, func(node *html.Node) bool {
		switch node.Type {
		case html.ElementNode:
			for _, attribute := range node.Attr {
				if attribute.Key == "href" || attribute.Key == "src" || attribute.Key == "action" || strings.HasPrefix(attribute.Key, "data-") {
					visit(attribute.Val)
				}
			}
		case html.TextNode:
			if node.Parent != nil && node.Parent.DataAtom == atom.Script {
				for _, raw := range pageURLPattern.FindAllString(node.Data, -1) {
					visit(raw)
				}
			}
		}
		return true
	})

	pageID := pageURL.Path + "?" + pageURL.RawQuery
	switch {
	case len(postings) == 1:
		return postings[0], true
	case len(postings) > 1:
		for _, reference := range postings {
			if strings.Contains(pageID, reference.PostingID) {
				return reference, true
			}
		}
		return jobboards.PostingReference{}, false
	}
	query := pageURL.Query()
	if id, board := query.Get("gh_jid"), boards[jobboards.Greenhouse]; id != "" && board != "" {
		return jobboards.PostingReference{Provider: jobboards.Greenhouse, BoardToken: board, PostingID: id}, true
	}
	if id, board := query.Get("ashby_jid"), boards[jobboards.Ashby]; id != "" && board != "" {
		return jobboards.PostingReference{Provider: jobboards.Ashby, BoardToken: board, PostingID: id}, true
	}
	return jobboards.PostingReference{}, false
}

// walk visits the node and its descendants in document order; visit returns
// false to skip a node's descendants.
func walk(node *html.Node, visit func(*html.Node) bool) {
	if !visit(node) {
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

// find returns the first element, in document order, that matches.
func find(node *html.Node, matches func(*html.Node) bool) *html.Node {
	var found *html.Node
	walk(node, func(current *html.Node) bool {
		if found != nil {
			return false
		}
		if current.Type == html.ElementNode && matches(current) {
			found = current
			return false
		}
		return true
	})
	return found
}

func getAttribute(node *html.Node, key string) string {
	value, _ := getAttributeEntry(node, key)
	return value
}

func getAttributeEntry(node *html.Node, key string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val, true
		}
	}
	return "", false
}

func isElement(name atom.Atom) func(*html.Node) bool {
	return func(node *html.Node) bool { return node.DataAtom == name }
}

// getText is the node's text, its spaces collapsed.
func getText(node *html.Node) string {
	var text strings.Builder
	walk(node, func(current *html.Node) bool {
		if current.Type == html.ElementNode && (current.DataAtom == atom.Script || current.DataAtom == atom.Style) {
			return false
		}
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
			text.WriteString(" ")
		}
		return true
	})
	return strings.Join(strings.Fields(text.String()), " ")
}

// readTitle is the posting's title as the page names it: its main content's
// first heading, else its og:title, else its <title>.
func readTitle(document *html.Node) string {
	root := findMainContent(document)
	if heading := find(root, isElement(atom.H1)); heading != nil {
		if title := getText(heading); title != "" {
			return title
		}
	}
	if meta := find(document, func(node *html.Node) bool {
		return node.DataAtom == atom.Meta && (getAttribute(node, "property") == "og:title" || getAttribute(node, "name") == "og:title")
	}); meta != nil {
		if title := strings.TrimSpace(getAttribute(meta, "content")); title != "" {
			return title
		}
	}
	if title := find(document, isElement(atom.Title)); title != nil {
		return getText(title)
	}
	return ""
}

// findMainContent is the element that holds the page's content: its <main>,
// else the element whose role is main, else its <article>, else its body.
func findMainContent(document *html.Node) *html.Node {
	for _, matches := range []func(*html.Node) bool{
		isElement(atom.Main),
		func(node *html.Node) bool { return getAttribute(node, "role") == "main" },
		isElement(atom.Article),
		isElement(atom.Body),
	} {
		if found := find(document, matches); found != nil {
			return found
		}
	}
	return document
}

// droppedElements are never a posting's text.
var droppedElements = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true, atom.Nav: true, atom.Aside: true,
	atom.Form: true, atom.Button: true, atom.Svg: true, atom.Iframe: true, atom.Dialog: true, atom.Select: true,
	atom.Input: true, atom.Textarea: true, atom.Head: true,
}

// droppedRoles are the landmarks around content, and the dialogs over it.
var droppedRoles = map[string]bool{"navigation": true, "banner": true, "contentinfo": true, "dialog": true, "alertdialog": true, "complementary": true}

// bannerPattern names the cookie and consent banners by their id or class.
var bannerPattern = regexp.MustCompile(`(?i)cookie|consent|gdpr|onetrust`)

// readMainText is the page's main content as Markdown, with no navigation,
// header, footer, cookie banner or form. A header or footer inside the main
// content, as an article's, is kept.
func readMainText(document *html.Node) string {
	root := findMainContent(document)
	isBody := root.DataAtom == atom.Body || root == document
	var markup strings.Builder
	var render func(node *html.Node)
	render = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			markup.WriteString(html.EscapeString(node.Data))
			return
		case html.ElementNode:
			if isDropped(node, isBody) {
				return
			}
			markup.WriteString("<" + node.Data + ">")
			defer markup.WriteString("</" + node.Data + ">")
		case html.CommentNode:
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			render(child)
		}
	}
	render(root)
	return strings.TrimSpace(textextract.ConvertHTMLToMarkdown(markup.String()))
}

func isDropped(node *html.Node, isBody bool) bool {
	if droppedElements[node.DataAtom] || droppedRoles[getAttribute(node, "role")] {
		return true
	}
	if isBody && (node.DataAtom == atom.Header || node.DataAtom == atom.Footer) {
		return true
	}
	if _, hidden := getAttributeEntry(node, "hidden"); hidden || getAttribute(node, "aria-hidden") == "true" {
		return true
	}
	return bannerPattern.MatchString(getAttribute(node, "id") + " " + getAttribute(node, "class"))
}
