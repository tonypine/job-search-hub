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
		TakeHome: &validTakeHome, RefuseHourlyWork: true,
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

var validTakeHome = store.TakeHome{
	Currency: "BRL", MinimumMonthly: 16000, TargetMonthly: 44000,
	CLT: store.HiringTakeHome{Share: 0.73, PaymentsPerYear: 13.33}, PJ: store.HiringTakeHome{Share: 0.82, PaymentsPerYear: 12},
	ForeignContractor: store.HiringTakeHome{Share: 0.84, PaymentsPerYear: 12},
}

func TestInvalidCriteriaAreRefused(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	for name, change := range map[string]func(*store.TakeHome){
		"a negative minimum": func(takeHome *store.TakeHome) { takeHome.MinimumMonthly = -1 },
		"no currency":        func(takeHome *store.TakeHome) { takeHome.Currency = "" },
		"a currency name":    func(takeHome *store.TakeHome) { takeHome.Currency = "reais" },
		"a share above one":  func(takeHome *store.TakeHome) { takeHome.PJ.Share = 1.2 },
		"no payments":        func(takeHome *store.TakeHome) { takeHome.CLT.PaymentsPerYear = 0 },
	} {
		takeHome := validTakeHome
		change(&takeHome)
		if _, err := hub.SaveJobCriteria(context.Background(), owner, store.JobCriteria{TakeHome: &takeHome}); err == nil {
			t.Errorf("%s: saved, want refused", name)
		}
	}
}
