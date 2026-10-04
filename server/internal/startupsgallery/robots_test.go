package startupsgallery

import "testing"

func TestRobotsRulesForTheHubDecideWhatItReads(t *testing.T) {
	file := []byte(`# A comment
User-agent: *
Disallow: /companies/
Allow: /companies/open-acme$
Disallow: /*.pdf$

User-agent: SomeOtherBot
Disallow: /

Sitemap: https://example.com/sitemap.xml
`)
	rules := readRobots(file, UserAgent)
	for path, want := range map[string]bool{
		"/categories/work-type/remote": true,
		"/companies/acme":              false,
		"/companies/open-acme":         true,
		"/companies/open-acme/jobs":    false,
		"/about/deck.pdf":              false,
		"/about/deck.pdf.html":         true,
	} {
		if got := rules.allows(path); got != want {
			t.Errorf("allows(%q) = %v, want %v", path, got, want)
		}
	}

	named := readRobots([]byte("User-agent: *\nAllow: /\n\nUser-agent: job-search-hub\nDisallow: /categories\n"), UserAgent)
	if named.allows("/categories/work-type/remote") || !named.allows("/companies/acme") {
		t.Errorf("the group naming the hub should apply instead of the * group: %+v", named)
	}
	if !readRobots([]byte("User-agent: *\nDisallow:\n"), UserAgent).allows(listPath) {
		t.Error("an empty Disallow allows everything")
	}
	if readRobots([]byte("User-agent: *\nDisallow: /\n"), UserAgent).allows(listPath) {
		t.Error("Disallow: / allows nothing")
	}
}
