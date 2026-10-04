package jobalerts_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/jobalerts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

var sentAt = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// makeIndeedLink builds a tracking link the way Indeed does: gzipped JSON
// holding the posting's address, base64url-encoded as the first path segment.
func makeIndeedLink(t *testing.T, postingURL string) string {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	writer.Write([]byte(`{"u":"` + postingURL + `","m":{"clickType":"viewjob"}}`))
	writer.Close()
	return "https://cts.indeed.com/v3/" + base64.RawURLEncoding.EncodeToString(compressed.Bytes()) + "/signature"
}

func TestAnIndeedAlertGivesItsPostingWithItsJobKey(t *testing.T) {
	link := makeIndeedLink(t, "https://br.indeed.com/rc/clk?jk=0a1b2c3d4e5f&from=email")
	alert := jobalerts.Alert{
		Sender: "Indeed <donotreply@match.indeed.com>", Subject: "Frontend Engineer (React) na empresa Acme Labs", SentAt: sentAt,
		Text: "Olá, Sam,\n\nCom base no seu currículo, acreditamos que você seja uma ótima opção para a vaga de Frontend Engineer (React).\n\n" +
			"Frontend Engineer (React)\nAcme Labs\nRemoto\nSalário: R$ 9.000 – R$ 12.000 por mês\n\nBenefícios:\n  - Horário flexível\n\n" +
			"Ver vaga: " + link + "\nCandidatar-se agora: " + link + "\n",
	}
	source, known := jobalerts.GetSource(alert.Sender)

	postings := jobalerts.ParsePostings(source, alert)

	if !known || source != store.JobSourceIndeed || len(postings) != 1 {
		t.Fatalf("source %q %v, postings %+v", source, known, postings)
	}
	posting := postings[0]
	if posting.ExternalID != "0a1b2c3d4e5f" || posting.URL != "https://br.indeed.com/viewjob?jk=0a1b2c3d4e5f" ||
		posting.Title != "Frontend Engineer (React)" || posting.CompanyName != "Acme Labs" || posting.Location != "Remoto" {
		t.Errorf("posting = %+v", posting)
	}
	if want := sentAt.Add(30 * 24 * time.Hour); posting.ExpiresAt == nil || !posting.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", posting.ExpiresAt, want)
	}
	if posting.Description == "" || !bytes.Contains([]byte(posting.Description), []byte("Salário: R$ 9.000")) {
		t.Errorf("description = %q; want the posting's block, pay included", posting.Description)
	}
}

// makeIndeedAlert builds Indeed's alert for one posting with the given title
// and company, in São Paulo.
func makeIndeedAlert(t *testing.T, title, company string) jobalerts.Alert {
	t.Helper()
	link := makeIndeedLink(t, "https://br.indeed.com/rc/clk?jk=9f8e7d6c5b4a&from=email")
	return jobalerts.Alert{
		Sender: "Indeed <donotreply@match.indeed.com>", Subject: title + " na empresa " + company, SentAt: sentAt,
		Text: title + "\n" + company + "\nSão Paulo, SP\n\nVer vaga: " + link + "\n",
	}
}

func TestAnIndeedAlertWhoseCompanyIsASloganTakesItFromTheTitle(t *testing.T) {
	alert := makeIndeedAlert(t, "Desenvolvedor Front-End Pleno - E-commerce| ACME SPORTS | São Paulo",
		"A seleção já está rolando na Acme Sports! Vem com a gente?")

	postings := jobalerts.ParsePostings(store.JobSourceIndeed, alert)

	if len(postings) != 1 {
		t.Fatalf("postings = %+v", postings)
	}
	if posting := postings[0]; posting.CompanyName != "Acme Sports" || posting.Title != "Desenvolvedor Front-End Pleno - E-commerce | São Paulo" ||
		posting.ExternalID != "9f8e7d6c5b4a" || posting.Location != "São Paulo, SP" {
		t.Errorf("posting = %+v; want the company from the title, in the slogan's casing, and the title without it", posting)
	}
}

func TestAnIndeedAlertKeepsItsTitleAndCompanyUnlessTheCompanyIsASloganNamingAPartOfTheTitle(t *testing.T) {
	for _, listing := range []struct{ title, company string }{
		{"Pessoa Desenvolvedora Front-end Sênior (React) | Remoto", "Acme Labs"},
		{"Desenvolvedor(a) Frontend Sênior | Remoto | PJ | Media Tech", "Acme Labs"},
		{"Desenvolvedor(a) Frontend Sênior | Remoto | PJ | Media Tech", "Media Tech Comunicação Digital"},
		{"Desenvolvedor Frontend | Acme Labs", "Venha construir o futuro com a gente!"},
		{"Desenvolvedor Frontend | Remoto", "Uma vaga 100% remoto na Acme Labs!"},
	} {
		postings := jobalerts.ParsePostings(store.JobSourceIndeed, makeIndeedAlert(t, listing.title, listing.company))

		if len(postings) != 1 || postings[0].Title != listing.title || postings[0].CompanyName != listing.company {
			t.Errorf("%q at %q gave %+v; want both kept", listing.title, listing.company, postings)
		}
	}
}

func TestLinkedInAlertsGiveEachPostingOnce(t *testing.T) {
	alert := jobalerts.Alert{
		Sender: "LinkedIn <jobs-noreply@linkedin.com>", Subject: "Senior Frontend Engineer at Globex", SentAt: sentAt,
		Text: "Jobs that match your profile\n\nBased on your title and location. Update:\nhttps://www.linkedin.com/comm/jobs/alerts\n  \n\n" +
			"Senior Frontend Engineer\nGlobex\nSão Paulo\nView job: https://www.linkedin.com/comm/jobs/view/111/?trackingId=a\n\n---------------------\n  \n" +
			"Staff Engineer\nInitrode\nRemote\nFast growing\nView job: https://www.linkedin.com/comm/jobs/view/222/?trackingId=b\n\n---------------------\n" +
			"People with similar roles applied to these jobs\n---------------------\n\n" +
			"Frontend Developer\nUmbrella · São Paulo, SPUmbrella · São Paulo, SP (Hybrid)\n\n\nView https://www.linkedin.com/comm/jobs/view/333/?lipi=x\n" +
			"Again: https://www.linkedin.com/comm/jobs/view/111/?trackingId=c\n",
	}

	postings := jobalerts.ParsePostings(store.JobSourceLinkedIn, alert)

	want := []store.JobPosting{
		{ExternalID: "111", Title: "Senior Frontend Engineer", CompanyName: "Globex", Location: "São Paulo", URL: "https://www.linkedin.com/jobs/view/111/"},
		{ExternalID: "222", Title: "Staff Engineer", CompanyName: "Initrode", Location: "Remote", URL: "https://www.linkedin.com/jobs/view/222/"},
		{ExternalID: "333", Title: "Frontend Developer", CompanyName: "Umbrella", Location: "São Paulo, SP (Hybrid)", URL: "https://www.linkedin.com/jobs/view/333/"},
	}
	if len(postings) != len(want) {
		t.Fatalf("postings = %+v", postings)
	}
	for index, posting := range postings {
		posting.ExpiresAt = nil
		if posting.ExternalID != want[index].ExternalID || posting.Title != want[index].Title || posting.CompanyName != want[index].CompanyName ||
			posting.Location != want[index].Location || posting.URL != want[index].URL {
			t.Errorf("posting %d = %+v, want %+v", index, posting, want[index])
		}
	}
}

func TestAGlassdoorAlertGivesItsCardsFromTheHTML(t *testing.T) {
	card := func(listingID, company, rating string, paragraphs ...string) string {
		markup := `<a href="https://www.glassdoor.com.br/partner/jobListing.htm?pos=1&amp;jobListingId=` + listingID + `&amp;utm_source=jobalert" target="_blank">` +
			`<table><tr><td><span><span>` + company + `</span>`
		if rating != "" {
			markup += `<span>&nbsp;<!-- -->` + rating + ` ★</span>`
		}
		markup += `</span></td></tr></table>`
		for _, paragraph := range paragraphs {
			markup += `<table><tr><td><p style="margin:0">` + paragraph + `</p></td></tr></table>`
		}
		return markup + `</a>`
	}
	alert := jobalerts.Alert{
		Sender: "Vagas do Glassdoor <noreply@glassdoor.com>", Subject: "Novas vagas: Brasil", SentAt: sentAt,
		HTML: `<html><body>` +
			card("901", "Hooli", "4.2", "Front-end Engineer | Payments", "São Paulo, São Paulo", "R$ 11 mil - R$ 13 mil <span>(estimativa)</span>", "3 dia(s)") +
			card("902", "Vandelay &amp; Co", "", "Desenvolvedor Pleno", "Trabalho remoto", "Candidatura rápida", "18h") +
			card("901", "Hooli", "4.2", "Front-end Engineer | Payments", "São Paulo, São Paulo", "3 dia(s)") +
			`<a href="https://www.glassdoor.com.br/profile/unsubscribeEmail.htm">Cancelar</a></body></html>`,
	}

	postings := jobalerts.ParsePostings(store.JobSourceGlassdoor, alert)

	if len(postings) != 2 {
		t.Fatalf("postings = %+v", postings)
	}
	first, second := postings[0], postings[1]
	if first.ExternalID != "901" || first.CompanyName != "Hooli" || first.Title != "Front-end Engineer | Payments" ||
		first.Location != "São Paulo, São Paulo" || first.URL != "https://www.glassdoor.com.br/job-listing/j?jl=901" ||
		first.Description != "R$ 11 mil - R$ 13 mil (estimativa)" || first.PublishedAt == nil || !first.PublishedAt.Equal(sentAt.Add(-72*time.Hour)) {
		t.Errorf("first = %+v", first)
	}
	if second.CompanyName != "Vandelay & Co" || second.Location != "Trabalho remoto" || second.PublishedAt == nil || !second.PublishedAt.Equal(sentAt.Add(-18*time.Hour)) {
		t.Errorf("second = %+v", second)
	}
}

func TestAGlassdoorCheckInGivesItsCards(t *testing.T) {
	card := func(listingID, title, company, location string) string {
		link := `<a href="https://www.glassdoor.com.br/partner/jobListing.htm?pos=101&amp;jobListingId=` + listingID + `&amp;utm_source=jobalert">`
		return link + `<img class="logo" alt="` + company + `"/></a></td><td valign="top">` + link +
			`<table><tr><td class="gd-title">` + title + `</td></tr></table><span class="gd-company">` + company + `<!-- --> - </span>` +
			`<span class="gd-place">` + location + `</span><table><tr><td>Candidatura rápida</td></tr></table></a>`
	}
	alert := jobalerts.Alert{
		Sender: "Vagas do Glassdoor <noreply@glassdoor.com>", Subject: "Como está sua busca de vagas de Engenheiro(a) De Software Front-end?", SentAt: sentAt,
		HTML: `<html><body><table><tr><td>` + card("801", "Especialista de Desenvolvimento Frontend", "Initech", "Campinas, São Paulo") +
			card("802", "Desenvolvedor Front-end", "Globex &amp; Filhos", "Trabalho remoto") + `</td></tr></table></body></html>`,
	}

	postings := jobalerts.ParsePostings(store.JobSourceGlassdoor, alert)

	if len(postings) != 2 {
		t.Fatalf("postings = %+v", postings)
	}
	if first := postings[0]; first.ExternalID != "801" || first.Title != "Especialista de Desenvolvimento Frontend" || first.CompanyName != "Initech" ||
		first.Location != "Campinas, São Paulo" || first.URL != "https://www.glassdoor.com.br/job-listing/j?jl=801" {
		t.Errorf("first = %+v", first)
	}
	if second := postings[1]; second.CompanyName != "Globex & Filhos" || second.Location != "Trabalho remoto" {
		t.Errorf("second = %+v", second)
	}
}

func TestOnlyAlertSendersHaveASource(t *testing.T) {
	for sender, want := range map[string]bool{
		"Indeed <donotreply@match.indeed.com>":       true,
		"LinkedIn <JOBS-NOREPLY@linkedin.com>":       true,
		"Vagas do Glassdoor <noreply@glassdoor.com>": true,
		"Shop <promotions@example.com>":              false,
		"not an address":                             false,
	} {
		if _, known := jobalerts.GetSource(sender); known != want {
			t.Errorf("GetSource(%q) known = %v, want %v", sender, known, want)
		}
	}
}
