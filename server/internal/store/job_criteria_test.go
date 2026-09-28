package store_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheCriteriaStartEmptyAndRoundTrip(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()

	empty, err := hub.GetJobCriteria(ctx)
	if err != nil || !reflect.DeepEqual(empty.Criteria, store.JobCriteria{}) {
		t.Fatalf("seeded criteria = %+v, %v; want empty", empty, err)
	}

	criteria := store.JobCriteria{
		Roles: []string{"Senior Full-Stack Engineer"}, SearchTerms: []string{"react"}, Technologies: []string{"TypeScript"},
		SeniorityLevels: []string{"Senior"}, HomeCountry: "Brazil", EligibleLocationTerms: []string{"Americas"}, IneligibleLocationTerms: []string{"must reside in the US"},
		MinimumYearlyPay: &store.MinimumYearlyPay{Amount: 120000, Currency: "USD"}, RefuseHourlyWork: true,
	}
	saved, err := hub.SaveJobCriteria(ctx, owner, criteria)
	if err != nil || !reflect.DeepEqual(saved.Criteria, criteria) {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	read, _ := hub.GetJobCriteria(ctx)
	if !reflect.DeepEqual(read.Criteria, criteria) {
		t.Fatalf("read = %+v", read.Criteria)
	}

	// Saving the same criteria again records nothing.
	if _, err := hub.SaveJobCriteria(ctx, owner, criteria); err != nil {
		t.Fatal(err)
	}
	var changes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM changes WHERE entity_type = 'job_criteria'`).Scan(&changes); err != nil || changes != 1 {
		t.Fatalf("criteria changes = %d, %v; want 1", changes, err)
	}
}

func TestInvalidCriteriaAreRefused(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	for name, pay := range map[string]store.MinimumYearlyPay{
		"negative pay":    {Amount: -1, Currency: "USD"},
		"no currency":     {Amount: 100000},
		"a currency name": {Amount: 100000, Currency: "dollars"},
	} {
		if _, err := hub.SaveJobCriteria(context.Background(), owner, store.JobCriteria{MinimumYearlyPay: &pay}); err == nil {
			t.Errorf("%s: saved, want refused", name)
		}
	}
}
