package textextract_test

import (
	"os"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

func TestTextIsReadFromPDFsHTMLAndPlainText(t *testing.T) {
	resume, err := os.ReadFile("testdata/made-up-resume.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, contentType, fileName string
		content                     []byte
		want                        string
	}{
		{"a PDF", "application/pdf", "resume.pdf", resume, "Senior Frontend Engineer"},
		{"a PDF sent as bytes", "application/octet-stream", "resume", resume, "React and TypeScript"},
		{"a saved page", "text/html; charset=utf-8", "team.html", []byte("<head><title>x</title></head><p>Hi <b>Sam</b>,</p><style>x{}</style><div>Next steps</div>"), "Hi Sam,\nNext steps"},
		{"plain text", "text/plain", "notes.txt", []byte("  Notes about Acme  \n"), "Notes about Acme"},
		{"markdown by its name", "", "notes.md", []byte("# Acme"), "# Acme"},
	} {
		text, err := textextract.Extract(test.contentType, test.fileName, test.content)
		if err != nil || !strings.Contains(text, test.want) {
			t.Errorf("%s: %q, %v; want it to contain %q", test.name, text, err, test.want)
		}
	}
}

func TestAFileWithoutReadableTextSaysWhy(t *testing.T) {
	if _, err := textextract.Extract("image/png", "photo.png", []byte{0x89, 'P', 'N', 'G'}); err == nil || !strings.Contains(err.Error(), "image/png") {
		t.Errorf("an image: %v", err)
	}
	if _, err := textextract.Extract("text/plain", "empty.txt", []byte("   ")); err == nil || !strings.Contains(err.Error(), "no text") {
		t.Errorf("an empty file: %v", err)
	}
	if _, err := textextract.Extract("application/pdf", "broken.pdf", []byte("%PDF-1.4 not really")); err == nil {
		t.Error("a broken PDF gave no error")
	}
}
