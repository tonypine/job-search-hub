package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

type recordedChange struct {
	actorKind string
	operation string
	before    []byte
	after     []byte
	sourceURL string
}

func changesFor(t *testing.T, pool *pgxpool.Pool, entityID uuid.UUID) []recordedChange {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT actor_kind, operation, before, after, source_url FROM changes WHERE entity_id = $1 ORDER BY id`, entityID)
	if err != nil {
		t.Fatalf("query changes: %v", err)
	}
	defer rows.Close()

	var recorded []recordedChange
	for rows.Next() {
		var entry recordedChange
		if err := rows.Scan(&entry.actorKind, &entry.operation, &entry.before, &entry.after, &entry.sourceURL); err != nil {
			t.Fatalf("scan change: %v", err)
		}
		recorded = append(recorded, entry)
	}
	return recorded
}

func TestCreateCompanyStoresTheCompanyAndOneChange(t *testing.T) {
	pool := testdatabase.New(t)
	companies := store.New(pool)

	company, created, err := companies.CreateCompany(context.Background(), owner, store.NewCompany{
		Name: "Stripe", Domain: "https://www.Stripe.com/jobs", WebsiteURL: "https://stripe.com", SourceURL: "https://stripe.com/about",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !created || company.Domain != "stripe.com" || company.Name != "Stripe" {
		t.Fatalf("created=%v company=%+v", created, company)
	}

	recorded := changesFor(t, pool, company.ID)
	if len(recorded) != 1 {
		t.Fatalf("got %d changes, want 1", len(recorded))
	}
	if recorded[0].actorKind != "owner" || recorded[0].operation != "create" || recorded[0].sourceURL != "https://stripe.com/about" || recorded[0].before != nil {
		t.Fatalf("change = %+v", recorded[0])
	}
}

func TestCreateCompanyReturnsTheStoredCompanyForTheSameDomain(t *testing.T) {
	pool := testdatabase.New(t)
	companies := store.New(pool)
	ctx := context.Background()

	first, _, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe", Domain: "Stripe.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, variant := range []string{"https://www.stripe.com/", "stripe.com", "http://STRIPE.com:443/careers?x=1"} {
		again, created, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe, Inc.", Domain: variant})
		if err != nil {
			t.Fatalf("create %q: %v", variant, err)
		}
		if created || again.ID != first.ID || again.Name != "Stripe" {
			t.Fatalf("%q: created=%v id=%v name=%q, want the first company unchanged", variant, created, again.ID, again.Name)
		}
	}
	if recorded := changesFor(t, pool, first.ID); len(recorded) != 1 {
		t.Fatalf("got %d changes, want only the create", len(recorded))
	}
}

func TestCreateCompanyRejectsAMissingNameOrAnInvalidDomain(t *testing.T) {
	companies := store.New(testdatabase.New(t))
	ctx := context.Background()

	if _, _, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: " ", Domain: "stripe.com"}); err == nil {
		t.Error("expected an error for a blank name")
	}
	if _, _, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe", Domain: "stripe"}); err == nil {
		t.Error("expected an error for a domain without a dot")
	}
}

func TestUpdateCompanyRecordsOnlyTheFieldsThatChanged(t *testing.T) {
	pool := testdatabase.New(t)
	companies := store.New(pool)
	ctx := context.Background()

	company, _, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe", Domain: "stripe.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	careersURL := "https://stripe.com/jobs"
	sameName := "Stripe"
	updated, err := companies.UpdateCompany(ctx, owner, company.ID, store.CompanyUpdate{
		Name: &sameName, CareersURL: &careersURL, SourceURL: "https://stripe.com",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.CareersURL != careersURL {
		t.Fatalf("careers_url = %q", updated.CareersURL)
	}

	recorded := changesFor(t, pool, company.ID)
	if len(recorded) != 2 || recorded[1].operation != "update" {
		t.Fatalf("changes = %+v", recorded)
	}
	var before, after map[string]string
	if err := json.Unmarshal(recorded[1].before, &before); err != nil {
		t.Fatalf("before: %v", err)
	}
	if err := json.Unmarshal(recorded[1].after, &after); err != nil {
		t.Fatalf("after: %v", err)
	}
	if len(after) != 1 || after["careers_url"] != careersURL || before["careers_url"] != "" {
		t.Fatalf("before=%v after=%v, want only careers_url", before, after)
	}

	if _, err := companies.UpdateCompany(ctx, owner, company.ID, store.CompanyUpdate{CareersURL: &careersURL}); err != nil {
		t.Fatalf("repeat update: %v", err)
	}
	if recorded := changesFor(t, pool, company.ID); len(recorded) != 2 {
		t.Fatalf("an update that changes nothing recorded a change: %d changes", len(recorded))
	}
}

func TestUpdateCompanyReportsAnUnknownCompany(t *testing.T) {
	companies := store.New(testdatabase.New(t))
	name := "Nobody"

	_, err := companies.UpdateCompany(context.Background(), owner, uuid.New(), store.CompanyUpdate{Name: &name})
	if !errors.Is(err, store.ErrCompanyNotFound) {
		t.Fatalf("err = %v, want ErrCompanyNotFound", err)
	}
}

func TestFindCompaniesMatchesNamesAndDomains(t *testing.T) {
	companies := store.New(testdatabase.New(t))
	ctx := context.Background()
	for _, input := range []store.NewCompany{{Name: "Stripe", Domain: "stripe.com"}, {Name: "Clio", Domain: "clio.com"}} {
		if _, _, err := companies.CreateCompany(ctx, owner, input); err != nil {
			t.Fatalf("create %s: %v", input.Name, err)
		}
	}

	for query, want := range map[string]string{"strip": "Stripe", "CLIO": "Clio", "https://www.clio.com/careers": "Clio"} {
		found, err := companies.FindCompanies(ctx, query)
		if err != nil {
			t.Fatalf("find %q: %v", query, err)
		}
		if len(found) != 1 || found[0].Name != want {
			t.Fatalf("find %q = %+v, want %s", query, found, want)
		}
	}
}

func TestGetCompanyByDomainNormalizesTheDomain(t *testing.T) {
	companies := store.New(testdatabase.New(t))
	ctx := context.Background()
	created, _, err := companies.CreateCompany(ctx, owner, store.NewCompany{Name: "Stripe", Domain: "stripe.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	found, err := companies.GetCompanyByDomain(ctx, "https://www.stripe.com/")
	if err != nil || found.ID != created.ID {
		t.Fatalf("found=%+v err=%v", found, err)
	}
	if _, err := companies.GetCompanyByDomain(ctx, "unknown.com"); !errors.Is(err, store.ErrCompanyNotFound) {
		t.Fatalf("err = %v, want ErrCompanyNotFound", err)
	}
}

func TestNormalizeDomain(t *testing.T) {
	for raw, want := range map[string]string{
		"Stripe.com":                     "stripe.com",
		"https://www.stripe.com/":        "stripe.com",
		"http://stripe.com:443/jobs?x=1": "stripe.com",
		"  www.Stripe.com. ":             "stripe.com",
		"jobs.lever.co":                  "jobs.lever.co",
	} {
		got, err := store.NormalizeDomain(raw)
		if err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "stripe", "not a domain", "https://"} {
		if got, err := store.NormalizeDomain(raw); err == nil {
			t.Errorf("NormalizeDomain(%q) = %q, want an error", raw, got)
		}
	}
}

func TestFoundViaIsStoredAndItsChangesRecorded(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()

	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com", FoundVia: "Board: startups.gallery"})
	if err != nil || company.FoundVia != "Board: startups.gallery" {
		t.Fatalf("create = %+v, err = %v", company, err)
	}
	referral := "Referral: a former colleague who interviewed there"
	updated, err := hub.UpdateCompany(ctx, owner, company.ID, store.CompanyUpdate{FoundVia: &referral})
	if err != nil || updated.FoundVia != referral {
		t.Fatalf("update = %+v, err = %v", updated, err)
	}
	recorded := changesFor(t, pool, company.ID)
	if len(recorded) != 2 || !strings.Contains(string(recorded[1].after), "found_via") {
		t.Fatalf("changes = %+v", recorded)
	}
}
