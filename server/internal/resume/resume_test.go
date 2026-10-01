package resume

import (
	"strings"
	"testing"
)

func TestDatesReadAsTheCVWritesThem(t *testing.T) {
	for _, test := range []struct{ start, end, want string }{
		{"2024-01", "", "Jan 2024 - Present"},
		{"2022-05", "2024-01", "May 2022 - Jan 2024"},
		{"2010", "2012", "2010 - 2012"},
	} {
		if got := formatDateRange(test.start, test.end); got != test.want {
			t.Errorf("%s to %s = %q, want %q", test.start, test.end, got, test.want)
		}
	}
	if got := formatDegree(Education{StudyType: "Technical Degree", Area: "Digital Media Design"}); got != "Technical Degree in Digital Media Design" {
		t.Errorf("degree = %q", got)
	}
	if GetBulletID(2, 1) != "w2h1" {
		t.Errorf("bullet id = %q", GetBulletID(2, 1))
	}
}

func TestTheHTMLHoldsEachSectionInTheCVsOrder(t *testing.T) {
	page, err := RenderHTML(Resume{
		Basics: Basics{Name: "Ada Lovelace", Label: "Senior Engineer", Location: Location{City: "London", Country: "UK"}, Summary: "Builds engines."},
		Work: []Work{{Name: "Acme", Position: "Engineer", Location: "Remote", StartDate: "2020-03",
			Highlights: []string{"Shipped <the> engine."}, Skills: []string{"Go", "React"}}},
		Education: []Education{{Institution: "Uni", StudyType: "BSc", Area: "Maths", StartDate: "2010", EndDate: "2013"}},
		Skills:    []Skill{{Name: "Go"}, {Name: "React"}},
		Languages: []Language{{Language: "English", Fluency: "Native"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)
	last := 0
	for _, want := range []string{"Ada Lovelace", "Senior Engineer", "London, UK", ">Summary<", "Builds engines.", ">Experience<", "Engineer",
		"Mar 2020 - Present", "Acme | Remote", "Shipped &lt;the&gt; engine.", "<strong>Skills:</strong> Go, React", ">Education<", "BSc in Maths",
		"2010 - 2013", ">Skills<", "Go, React", ">Languages<", "English: Native"} {
		index := strings.Index(text[last:], want)
		if index < 0 {
			t.Fatalf("%q missing after position %d", want, last)
		}
		last += index
	}
}
