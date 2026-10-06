package textextract_test

import (
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

func TestHTMLBecomesMarkdown(t *testing.T) {
	for _, test := range []struct {
		name, markup, want string
	}{
		{"headings of every level", "<h1>Acme</h1><p>We build tools.</p><h4>What you'll do</h4><p>Ship.</p>",
			"### Acme\n\nWe build tools.\n\n### What you'll do\n\nShip."},
		{"a heading's line breaks and bold", "<h2>\n  <strong>About</strong><br>the role\n</h2>", "### About the role"},
		{"a list", "<p>You have:</p>\n<ul>\n  <li>React</li>\n  <li>TypeScript</li>\n</ul>\n<p>Thanks.</p>",
			"You have:\n\n- React\n- TypeScript\n\nThanks."},
		{"an ordered list", "<ol><li>Apply</li><li>Talk to us</li></ol>", "1. Apply\n2. Talk to us"},
		{"a nested list", "<ul><li>Frontend<ul><li>React</li><li>Vue</li></ul></li><li>Backend</li></ul>",
			"- Frontend\n  - React\n  - Vue\n- Backend"},
		{"an ordered list in a list", "<ul><li>Steps<ol><li>Call</li><li>Code</li></ol></li></ul>", "- Steps\n  1. Call\n  2. Code"},
		{"an item's paragraphs and breaks", "<ul><li><p>Ship</p><p>daily<br>and safely</p></li></ul>", "- Ship daily and safely"},
		{"an empty item", "<ul><li> </li><li>React</li></ul>", "- React"},
		{"bold", "<p>Pay: <b>$100k</b> a year, <strong>remote </strong>first.</p>", "Pay: **$100k** a year, **remote** first."},
		{"bold split in runs", "<p><strong>Requirements</strong><strong>:</strong> React</p>", "**Requirements:** React"},
		{"nested bold", "<b>Fully <strong>remote</strong> team</b>", "**Fully remote team**"},
		{"bold across a line break", "<b>Remote<br>Brazil</b>", "**Remote**\n**Brazil**"},
		{"empty bold", "<p>Hi<b> </b>there</p>", "Hi there"},
		{"a bold item", "<ul><li><b>React</b> and Go</li></ul>", "- **React** and Go"},
		{"links keep their text", `<p>Read <a href="https://acme.example/blog">our blog</a>.</p>`, "Read our blog."},
		{"entities", "<p>R&amp;D &lt;3 &quot;remote&quot; caf&eacute;&nbsp;team</p>", `R&D <3 "remote" café team`},
		{"scripts and styles", "<style>p{color:red}</style><p>Hi</p><script>alert(1)</script><p>There</p>", "Hi\n\nThere"},
		{"blank runs", "<p>One</p><p>&nbsp;</p><p></p><br><br><br><div><div>Two</div></div>", "One\n\nTwo"},
		{"line breaks in text", "First line\nsecond line\n\n\n\nNext", "First line\nsecond line\nNext"},
		{"plain text", "  Just text.  ", "Just text."},
		{"a table", "<table><tr><td>Pay</td><td>$100k</td></tr><tr><td>Where</td><td>Remote</td></tr></table>", "Pay $100k\nWhere Remote"},
		{"nothing", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := textextract.ConvertHTMLToMarkdown(test.markup); got != test.want {
				t.Errorf("ConvertHTMLToMarkdown(%q) =\n%q\nwant\n%q", test.markup, got, test.want)
			}
		})
	}
}

func TestMarkdownBecomesPlainWords(t *testing.T) {
	for _, test := range []struct {
		name, markdown, want string
	}{
		{"a heading", "### What you'll do", "What you'll do"},
		{"bullets", "- React\n  - Vue\n* Go", "React\nVue\nGo"},
		{"an ordered list", "1. Apply\n2. Talk", "Apply\nTalk"},
		{"bold", "Pay: **$100k** a year", "Pay: $100k a year"},
		{"a bold item", "- **React** required", "React required"},
		{"a dash inside a line", "Full-time - remote", "Full-time - remote"},
		{"a hash inside a line", "C# and F#", "C# and F#"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := textextract.ConvertMarkdownToText(test.markdown); got != test.want {
				t.Errorf("ConvertMarkdownToText(%q) = %q, want %q", test.markdown, got, test.want)
			}
		})
	}
}
