package mcptools

import "testing"

func TestACareersPageRoleKeepsItsHeadingsAndListsAsMarkdown(t *testing.T) {
	postings, err := convertCareersPageJobs([]careersPageJob{
		{Title: "Frontend Engineer", URL: "https://acme.example/careers/fe", Description: "### What you'll do\n\n- Build the app\n- Talk to customers"},
		{Title: "Designer", URL: "https://acme.example/careers/design",
			Description: "<h2>What you'll do</h2><p>Draw <strong>flows</strong>.</p><ul><li>Figma</li></ul>"},
	})
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	if got, want := postings[0].Description, "### What you'll do\n\n- Build the app\n- Talk to customers"; got != want {
		t.Errorf("a Markdown description = %q, want it kept as %q", got, want)
	}
	if got, want := postings[1].Description, "### What you'll do\n\nDraw **flows**.\n\n- Figma"; got != want {
		t.Errorf("an HTML description = %q, want %q", got, want)
	}
}
