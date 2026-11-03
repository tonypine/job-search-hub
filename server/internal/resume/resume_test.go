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

func TestTheHeaderLinksThePhoneEmailAndProfilesAfterTheLocation(t *testing.T) {
	page, err := RenderHTML(Resume{Basics: Basics{
		Name: "Ada Lovelace", Label: "Senior Engineer", Location: Location{City: "London", Country: "UK"},
		Phone: "+44 20 7946 0000", Email: "ada@example.com",
		Profiles: []Profile{{Network: "LinkedIn", Username: "ada-example", URL: "https://www.linkedin.com/in/ada-example/"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := `<p class="location">London, UK · <a href="tel:&#43;442079460000">&#43;44 20 7946 0000</a> · ` +
		`<a href="mailto:ada@example.com">ada@example.com</a> · ` +
		`<a href="https://www.linkedin.com/in/ada-example/">linkedin.com/in/ada-example</a></p>`
	if !strings.Contains(string(page), want) {
		t.Errorf("the contact line is missing:\nwant %s\nin %s", want, page)
	}
	if !strings.Contains(string(page), ".location a { color: inherit; text-decoration: none; }") {
		t.Error("the contact links must read as the location line, with no underline or link colour")
	}
}

func TestAHeaderWithoutContactDetailsHasNoEmptySeparators(t *testing.T) {
	for _, test := range []struct {
		name   string
		basics Basics
		want   string
	}{
		{"location only", Basics{Name: "Ada", Location: Location{City: "London", Country: "UK"}}, `<p class="location">London, UK</p>`},
		{"email only", Basics{Name: "Ada", Email: "ada@example.com"}, `<p class="location"><a href="mailto:ada@example.com">ada@example.com</a></p>`},
		{"a profile with no web address", Basics{Name: "Ada", Profiles: []Profile{{Network: "LinkedIn", Username: "ada-example", URL: "javascript:alert(1)"}}},
			`<p class="location">ada-example</p>`},
	} {
		page, err := RenderHTML(Resume{Basics: test.basics})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(page), test.want) {
			t.Errorf("%s: want %s in %s", test.name, test.want, page)
		}
	}
	page, err := RenderHTML(Resume{Basics: Basics{Name: "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), `class="location"`) || strings.Contains(string(page), "·") {
		t.Errorf("a CV with no location or contact details has a contact line: %s", page)
	}
}

func TestThePhoneDialsItsDigits(t *testing.T) {
	for _, test := range []struct{ phone, want string }{
		{"+55 (11) 91234-5678", "+5511912345678"},
		{"020 7946 0000", "02079460000"},
		{"call me", ""},
		{"+", ""},
	} {
		if got := buildPhoneNumber(test.phone); got != test.want {
			t.Errorf("%q dials %q, want %q", test.phone, got, test.want)
		}
	}
}

// TestALongRoleSplitsOnlyBetweenItsMiddleBullets checks the markup WebKit
// paginates by: a role's heading, place line and first bullet in one .keep,
// its middle bullets free to split, and its last bullet with its skills line
// in another .keep.
func TestALongRoleSplitsOnlyBetweenItsMiddleBullets(t *testing.T) {
	highlights := []string{"b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9"}
	page, err := RenderHTML(Resume{Basics: Basics{Name: "Ada"}, Work: []Work{
		{Name: "Acme", Position: "Engineer", StartDate: "2020-03", Highlights: highlights, Skills: []string{"Go"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(page)), " ")
	for _, want := range []string{
		`<div class="entry"> <div class="keep"> <h2>Experience</h2> <div class="heading"><strong>Engineer</strong>`,
		`<p class="place">Acme</p> <ul><li>b1</li></ul> </div>`,
		`<ul class="rest"><li>b2</li><li>b3</li><li>b4</li><li>b5</li><li>b6</li><li>b7</li><li>b8</li></ul>`,
		`<div class="keep"> <ul class="rest"><li>b9</li></ul> <p class="skills"><strong>Skills:</strong> Go</p> </div>`,
		`.entry { margin-bottom: 9pt; break-inside: auto; }`,
		`.keep, li { break-inside: avoid; }`,
		`ul.rest { margin-top: 0; }`,
		`break-after: avoid; }`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in %s", want, text)
		}
	}
	if strings.Count(text, "<li>") != len(highlights) || strings.Count(text, "Skills:") != 1 {
		t.Errorf("each bullet and the skills line print once: %s", text)
	}
}

func TestAShortRoleKeepsItsBulletsWithItsHeading(t *testing.T) {
	for _, test := range []struct {
		highlights []string
		want       string
	}{
		{nil, `<p class="place">Acme</p> <p class="skills"><strong>Skills:</strong> Go</p> </div> </div>`},
		{[]string{"only"}, `<p class="place">Acme</p> <ul><li>only</li></ul> <p class="skills"><strong>Skills:</strong> Go</p> </div> </div>`},
		{[]string{"first", "last"}, `<ul><li>first</li></ul> </div> <div class="keep"> <ul class="rest"><li>last</li></ul> <p class="skills">`},
	} {
		page, err := RenderHTML(Resume{Basics: Basics{Name: "Ada"}, Work: []Work{
			{Name: "Acme", Position: "Engineer", StartDate: "2020-03", Highlights: test.highlights, Skills: []string{"Go"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(string(page)), " ")
		if !strings.Contains(text, test.want) {
			t.Errorf("%d bullets: missing %s in %s", len(test.highlights), test.want, text)
		}
		if strings.Contains(text, `class="rest"></ul>`) || strings.Count(text, "Skills:") != 1 {
			t.Errorf("%d bullets: an empty list or a doubled skills line: %s", len(test.highlights), text)
		}
	}
}

func TestEachSectionHeadingStaysWithItsFirstBlock(t *testing.T) {
	page, err := RenderHTML(Resume{
		Basics:    Basics{Name: "Ada", Summary: "Builds engines."},
		Work:      []Work{{Name: "Acme", Position: "Engineer", StartDate: "2020"}, {Name: "Beta", Position: "Lead", StartDate: "2022"}},
		Education: []Education{{Institution: "Uni", StudyType: "BSc", StartDate: "2010"}, {Institution: "College", StudyType: "MSc", StartDate: "2014"}},
		Skills:    []Skill{{Name: "Go"}},
		Languages: []Language{{Language: "English", Fluency: "Native"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(page)), " ")
	for _, want := range []string{
		`<section class="keep"> <h2>Summary</h2>`,
		`<div class="keep"> <h2>Experience</h2> <div class="heading"><strong>Engineer</strong>`,
		`<div class="entry keep"> <h2>Education</h2> <div class="heading"><strong>BSc</strong>`,
		`<section class="keep"> <h2>Skills</h2>`,
		`<section class="keep"> <h2>Languages</h2>`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in %s", want, text)
		}
	}
	if strings.Count(text, "<h2>Experience</h2>") != 1 || strings.Count(text, "<h2>Education</h2>") != 1 {
		t.Errorf("each heading prints once: %s", text)
	}
}
