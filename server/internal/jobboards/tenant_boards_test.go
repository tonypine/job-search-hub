package jobboards

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func startTenantBoards(t *testing.T) *Verifier {
	t.Helper()
	routes := http.NewServeMux()
	routes.HandleFunc("GET /api/offers/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"offers":[
			{"id":101,"title":"Senior Frontend Engineer","status":"published","careers_url":"https://acme.recruitee.com/o/senior-frontend-engineer",
			 "description":"<h2>About</h2><p>Build the app.</p>","requirements":"<ul><li>React</li></ul>","remote":true,"hybrid":false,
			 "employment_type_code":"fulltime_permanent","department":"Engineering","published_at":"2026-09-25 18:00:43 UTC",
			 "locations":[{"city":"São Paulo ","state":"São Paulo","country":"Brazil"},{"city":"Lisbon","state":"","country":"Portugal"}],
			 "salary":{"min":"8000","max":"12000","period":"month","currency":"USD"}},
			{"id":102,"title":"Old Role","status":"closed","careers_url":"https://acme.recruitee.com/o/old","salary":{"min":null,"max":null}}]}`)
	})
	routes.HandleFunc("GET /careers/list", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"meta":{"totalCount":1},"result":[{"id":"7","jobOpeningName":"Backend Engineer"}]}`)
	})
	routes.HandleFunc("GET /careers/7/detail", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"result":{"jobOpening":{"jobOpeningName":"Backend Engineer","jobOpeningStatus":"Open",
			"jobOpeningShareUrl":"https://acme.bamboohr.com/careers/7","description":"<h2>Role</h2><p>Write <strong>Go</strong>.</p><ol><li>Design</li><li>Ship</li></ol>",
			"departmentLabel":"Platform","employmentStatusLabel":"Full-Time","location":{"city":null,"state":null},
			"atsLocation":{"country":"Brazil","state":null,"city":null},"isRemote":null,"locationType":"1","datePosted":"2026-09-16"}}}`)
	})
	routes.HandleFunc("GET /v1/companies/acme/postings", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"offset":0,"limit":100,"totalFound":1,"content":[{"id":"11","name":"Web Developer"}]}`)
	})
	routes.HandleFunc("GET /v1/companies/acme/postings/11", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"11","name":"Web Developer","active":true,"postingUrl":"https://jobs.smartrecruiters.com/Acme/11-web-developer",
			"releasedDate":"2026-08-05T13:41:17.883Z","department":null,"typeOfEmployment":{"label":"Full-time"},
			"location":{"city":"Joinville","region":"SC","country":"br","remote":false,"hybrid":true,"fullLocation":"Joinville, SC, Brazil"},
			"jobAd":{"sections":{"companyDescription":{"text":"<p>We make tools.</p>"},"jobDescription":{"text":"<h3>The role</h3><p>Ship features.</p>"},
			"qualifications":{"text":"<ul><li>TypeScript</li></ul>"},"additionalInformation":{"text":""}}}}`)
	})
	routes.HandleFunc("GET /v1/companies/nobody/postings", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"offset":0,"limit":100,"totalFound":0,"content":[]}`)
	})
	routes.HandleFunc("GET /xml", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><workzag-jobs><position><id>55</id><name>Fullstack Developer</name>
			<office>Berlin</office><additionalOffices><office>Remote</office></additionalOffices><department>Tech</department>
			<schedule>full-time</schedule><createdAt>2026-07-08T12:29:56+00:00</createdAt>
			<jobDescriptions><jobDescription><name>Your tasks</name><value><![CDATA[<p>Build &amp; run.</p><ul><li>Go</li></ul>]]></value></jobDescription></jobDescriptions>
			<salaryInformation><min>60000.00</min><max>80000.00</max><currencyCode>EUR</currencyCode><type>yearly</type></salaryInformation>
			</position></workzag-jobs>`)
	})
	routes.HandleFunc("GET /postings.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"9","title":"Senior Software Engineer - React / Node.js","url":"https://acme.pinpointhq.com/en/postings/9",
			"description":"<div>Lead the web app.</div>","key_responsibilities_header":"Qualifications","key_responsibilities":"<ul><li>5+ years</li></ul>",
			"skills_knowledge_expertise_header":null,"skills_knowledge_expertise":null,"benefits_header":null,"benefits":null,
			"workplace_type":"remote","employment_type_text":"Full Time","compensation_minimum":null,"compensation_maximum":null,
			"compensation_visible":true,"location":{"name":"Remote","city":"Remote","province":""},"job":{"department":{"name":"Engineering"}}}]}`)
	})
	nextPage := func(pageProps string) string {
		return `<html><body><div id="__next"></div><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":` + pageProps +
			`},"page":"/"}</script></body></html>`
	}
	routes.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, nextPage(`{"subdomain":"acme","jobs":[
			{"id":7,"title":"Desenvolvedor(a) Front-end Sênior","type":"vacancy_type_effective","workplace":{"workplaceType":"remote"}},
			{"id":8,"title":"Banco de Talentos","type":"vacancy_type_talent_pool","workplace":{"workplaceType":"remote"}}]}`))
	})
	routes.HandleFunc("GET /jobs/7", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, nextPage(`{"job":{"id":"7","name":"Desenvolvedor(a) Front-end Sênior","description":"<h2>Sobre</h2><p>Construa o app.</p>",
			"responsibilities":"<ul><li>React</li></ul>","prerequisites":"","relevantExperiences":null,"addressCity":"Curitiba",
			"addressState":"Paraná","addressCountry":"Brasil","workplaceType":"remote","jobType":"vacancy_type_effective",
			"status":"published","publishedAt":"2026-09-20T12:00:00.000Z"}}`))
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return &Verifier{
		HTTPClient: &http.Client{Timeout: time.Second}, RecruiteeAPIBase: server.URL, BambooHRAPIBase: server.URL,
		SmartRecruitersAPIBase: server.URL, PersonioAPIBase: server.URL, PinpointAPIBase: server.URL, GupyBase: server.URL,
	}
}

func TestTenantBoardsAreReadWithTheirText(t *testing.T) {
	verifier := startTenantBoards(t)
	ctx := context.Background()
	cases := []struct {
		provider string
		want     store.JobPosting
	}{
		{Recruitee, store.JobPosting{ExternalID: "101", Title: "Senior Frontend Engineer", URL: "https://acme.recruitee.com/o/senior-frontend-engineer",
			Location: "São Paulo, São Paulo, Brazil", WorkplaceType: "Remote", Description: "### About\n\nBuild the app.\n\n- React",
			BoardFacts: store.BoardFacts{EmploymentType: "Full-time", Department: "Engineering", OtherLocations: []string{"Lisbon, Portugal"}}}},
		{BambooHR, store.JobPosting{ExternalID: "7", Title: "Backend Engineer", URL: "https://acme.bamboohr.com/careers/7", Location: "Brazil",
			WorkplaceType: "Remote", Description: "### Role\n\nWrite **Go**.\n\n1. Design\n2. Ship", BoardFacts: store.BoardFacts{EmploymentType: "Full-Time", Department: "Platform"}}},
		{SmartRecruiters, store.JobPosting{ExternalID: "11", Title: "Web Developer", URL: "https://jobs.smartrecruiters.com/Acme/11-web-developer",
			Location: "Joinville, SC, Brazil", WorkplaceType: "Hybrid", Description: "We make tools.\n\n### The role\n\nShip features.\n\n- TypeScript",
			BoardFacts: store.BoardFacts{EmploymentType: "Full-time"}}},
		{Personio, store.JobPosting{ExternalID: "55", Title: "Fullstack Developer", Location: "Berlin", WorkplaceType: "Remote",
			Description: "### Your tasks\n\nBuild & run.\n\n- Go", BoardFacts: store.BoardFacts{EmploymentType: "Full-time", Department: "Tech", OtherLocations: []string{"Remote"}}}},
		{Gupy, store.JobPosting{ExternalID: "7", Title: "Desenvolvedor(a) Front-end Sênior", Location: "Curitiba, Paraná, Brasil",
			WorkplaceType: "Remote", Description: "### Sobre\n\nConstrua o app.\n\n- React"}},
		{Pinpoint, store.JobPosting{ExternalID: "9", Title: "Senior Software Engineer - React / Node.js", URL: "https://acme.pinpointhq.com/en/postings/9",
			Location: "Remote", WorkplaceType: "Remote", Description: "Lead the web app.\n\n### Qualifications\n\n- 5+ years",
			BoardFacts: store.BoardFacts{EmploymentType: "Full Time", Department: "Engineering"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.provider, func(t *testing.T) {
			postings, err := verifier.FetchPostings(ctx, testCase.provider, "acme")
			if err != nil || len(postings) != 1 {
				t.Fatalf("postings = %+v, %v; want one open posting", postings, err)
			}
			got, want := postings[0], testCase.want
			switch testCase.provider {
			case Personio:
				want.URL = verifier.PersonioAPIBase + "/job/55"
			case Gupy:
				want.URL = verifier.GupyBase + "/jobs/7"
			}
			if got.ExternalID != want.ExternalID || got.Title != want.Title || got.URL != want.URL || got.Location != want.Location ||
				got.WorkplaceType != want.WorkplaceType || got.Description != want.Description || got.EmploymentType != want.EmploymentType ||
				got.Department != want.Department || fmt.Sprint(got.OtherLocations) != fmt.Sprint(want.OtherLocations) || got.PublishedAt == nil && testCase.provider != Pinpoint {
				t.Errorf("posting =\n%+v\nwant\n%+v", got, want)
			}
		})
	}

	recruitee, _ := verifier.FetchPostings(ctx, Recruitee, "acme")
	if pay := recruitee[0].Pay; pay == nil || pay.Ranges[0].Min != 8000 || pay.Ranges[0].Max != 12000 || pay.Ranges[0].Interval != "month" {
		t.Errorf("Recruitee pay = %+v", pay)
	}
	personio, _ := verifier.FetchPostings(ctx, Personio, "acme")
	if pay := personio[0].Pay; pay == nil || pay.Ranges[0].Max != 80000 || pay.Ranges[0].Currency != "EUR" || pay.Ranges[0].Interval != "year" {
		t.Errorf("Personio pay = %+v", pay)
	}
	if postings, err := verifier.FetchPostings(ctx, SmartRecruiters, "nobody"); err != nil || len(postings) != 0 {
		t.Errorf("a company SmartRecruiters doesn't know = %+v, %v; want no postings", postings, err)
	}
}

func TestTenantBoardsListTheirTitlesInOneRequest(t *testing.T) {
	verifier := startTenantBoards(t)
	want := map[string]string{
		Recruitee: "[Senior Frontend Engineer Old Role]", BambooHR: "[Backend Engineer]", SmartRecruiters: "[Web Developer]",
		Personio: "[Fullstack Developer]", Pinpoint: "[Senior Software Engineer - React / Node.js]", Gupy: "[Desenvolvedor(a) Front-end Sênior]",
	}
	for provider, titles := range want {
		got, err := verifier.ListPostingTitles(context.Background(), provider, "acme")
		if err != nil || fmt.Sprint(got) != titles {
			t.Errorf("%s titles = %v, %v; want %s", provider, got, err, titles)
		}
	}
}

func TestABambooHRNameThatRedirectsIsNoBoard(t *testing.T) {
	asked := 0
	marketing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(marketing.Close)
	tenant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, marketing.URL, http.StatusFound)
	}))
	t.Cleanup(tenant.Close)
	verifier := &Verifier{HTTPClient: &http.Client{Timeout: time.Second}, BambooHRAPIBase: tenant.URL}
	if _, err := verifier.FetchPostings(context.Background(), BambooHR, "nobody"); !errors.Is(err, ErrPostingAPIOff) {
		t.Errorf("err = %v, want ErrPostingAPIOff", err)
	}
	if _, err := verifier.ListPostingTitles(context.Background(), BambooHR, "nobody"); !errors.Is(err, ErrPostingAPIOff) {
		t.Errorf("listing titles: err = %v, want ErrPostingAPIOff", err)
	}
	if asked != 0 {
		t.Errorf("the redirect was followed to BambooHR's own site %d times", asked)
	}
}

func TestATokenThatCantBeASubdomainNamesNoBoard(t *testing.T) {
	verifier := NewVerifier()
	if _, err := verifier.FetchPostings(context.Background(), Recruitee, "Acme Labs"); !errors.Is(err, ErrPostingAPIOff) {
		t.Errorf("err = %v, want ErrPostingAPIOff", err)
	}
}

func TestAProviderIsLeftAloneUntilItsRetryAfter(t *testing.T) {
	asked := 0
	limiting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(limiting.Close)
	verifier := &Verifier{HTTPClient: &http.Client{Timeout: time.Second}, RecruiteeAPIBase: limiting.URL, PinpointAPIBase: limiting.URL}
	ctx := context.Background()
	for range 3 {
		if _, err := verifier.ListPostingTitles(ctx, Recruitee, "acme"); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("err = %v, want ErrRateLimited", err)
		}
	}
	if asked != 1 {
		t.Errorf("Recruitee was asked %d times within its Retry-After; want once", asked)
	}
	if _, err := verifier.ListPostingTitles(ctx, Pinpoint, "acme"); !errors.Is(err, ErrRateLimited) || asked != 2 {
		t.Errorf("another provider: err = %v, asked = %d; want it asked", err, asked)
	}
}

func TestRetryAfterIsReadInSecondsOrAsADate(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{"85612": 85612 * time.Second, "Thu, 01 Oct 2026 13:00:00 GMT": time.Hour, "": defaultRetryAfter, "soon": defaultRetryAfter}
	for header, want := range cases {
		if got := getRetryAfter(header, now); got != want {
			t.Errorf("getRetryAfter(%q) = %s, want %s", header, got, want)
		}
	}
}

func TestRemoteOKsFeedIsReadPastItsTerms(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tag") != "react" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `[{"last_updated":1790792682,"legal":"Please link back."},
			{"id":"1137451","date":"2026-09-30T18:24:42+00:00","company":"Acme","position":"Senior Frontend Engineer","tags":["react"],
			 "description":"<h2>Role</h2><p>Build the <b>app</b>.</p><ul><li>React</li></ul>","location":"Worldwide","salary_min":90000,"salary_max":120000,
			 "url":"https://remoteOK.com/remote-jobs/1137451"},
			{"id":"1137452","date":"2026-09-30T18:00:00+00:00","company":"Globex","position":"Designer","description":"","location":"",
			 "salary_min":0,"salary_max":0,"url":"https://remoteOK.com/remote-jobs/1137452"}]`)
	}))
	t.Cleanup(feed.Close)
	verifier := &Verifier{HTTPClient: &http.Client{Timeout: time.Second}, RemoteOKAPIBase: feed.URL}

	postings, err := verifier.FetchRemoteOKPostings(context.Background(), "react")
	if err != nil || len(postings) != 2 {
		t.Fatalf("postings = %+v, %v", postings, err)
	}
	first := postings[0]
	if first.ExternalID != "1137451" || first.CompanyName != "Acme" || first.Title != "Senior Frontend Engineer" || first.Location != "Worldwide" ||
		first.WorkplaceType != "Remote" || first.Description != "### Role\n\nBuild the **app**.\n\n- React" || first.ExpiresAt == nil || !first.ExpiresAt.After(time.Now()) ||
		first.PublishedAt == nil ||
		first.Pay == nil || first.Pay.Ranges[0].Min != 90000 || first.Pay.Ranges[0].Currency != "USD" || first.Pay.Ranges[0].Interval != "year" {
		t.Errorf("first = %+v", first)
	}
	if postings[1].Pay != nil {
		t.Errorf("a posting with zero pay has pay %+v", postings[1].Pay)
	}
}
