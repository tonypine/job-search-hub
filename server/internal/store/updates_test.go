package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestAnUpdateAboutAJobIsUnseenOnTheJobItsCompanyAndItsCardUntilSeen(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Engineer", URL: "https://acme.com/jobs/1"})
	if _, _, err := hub.AddApplication(ctx, owner, store.ApplicationInput{JobID: &job.ID}); err != nil {
		t.Fatal(err)
	}

	update, err := hub.RecordUpdate(ctx, store.NewUpdate{Kind: "reply", Title: "Acme replied", JobID: &job.ID, SourceURL: "https://mail.google.com/x"})
	if err != nil || update.CompanyID == nil || *update.CompanyID != company.ID || *update.JobTitle != "Engineer" || *update.CompanyName != "Acme" {
		t.Fatalf("update = %+v, %v; want the job's company filled in", update, err)
	}
	unseenCounts := func() (int, int, int) {
		jobs, _, _ := hub.ListJobs(ctx, store.JobFilter{})
		companies, _ := hub.ListCompanySummaries(ctx)
		cards, _ := hub.ListPipelineCards(ctx)
		return jobs[0].UnseenUpdates, companies[0].UnseenUpdates, cards[0].UnseenUpdates
	}
	if onJob, onCompany, onCard := unseenCounts(); onJob != 1 || onCompany != 1 || onCard != 1 {
		t.Fatalf("unseen on job, company, card = %d, %d, %d; want 1 each", onJob, onCompany, onCard)
	}
	if details, _ := hub.GetJobDetails(ctx, job.ID); details.UnseenUpdates != 1 {
		t.Fatalf("details unseen = %d", details.UnseenUpdates)
	}

	if marked, err := hub.MarkUpdatesSeen(ctx, store.UpdateSelection{JobID: &job.ID}); err != nil || marked != 1 {
		t.Fatalf("mark the job seen = %d, %v", marked, err)
	}
	if onJob, onCompany, onCard := unseenCounts(); onJob != 0 || onCompany != 0 || onCard != 0 {
		t.Fatalf("after seeing the job: %d, %d, %d; want none", onJob, onCompany, onCard)
	}
}

func TestUpdatesListNewestFirstWithTheUnseenTotal(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	first, _ := hub.RecordUpdate(ctx, store.NewUpdate{Kind: "recruiter", Title: "A recruiter wrote", CompanyID: &company.ID})
	if _, err := hub.RecordUpdate(ctx, store.NewUpdate{Kind: "note", Title: "Weekly sign-in expired"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.RecordUpdate(ctx, store.NewUpdate{Kind: "", Title: "no kind"}); err == nil {
		t.Fatal("an update without a kind was recorded")
	}

	list, err := hub.ListUpdates(ctx, 10, false)
	if err != nil || len(list.Updates) != 2 || list.Updates[0].Title != "Weekly sign-in expired" || list.UnseenCount != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err := hub.MarkUpdatesSeen(ctx, store.UpdateSelection{IDs: []uuid.UUID{first.ID}}); err != nil {
		t.Fatal(err)
	}
	unseen, _ := hub.ListUpdates(ctx, 10, true)
	if len(unseen.Updates) != 1 || unseen.UnseenCount != 1 {
		t.Fatalf("unseen = %+v", unseen)
	}
	if _, err := hub.MarkUpdatesSeen(ctx, store.UpdateSelection{}); err == nil {
		t.Fatal("an empty selection marked updates")
	}
	if marked, _ := hub.MarkUpdatesSeen(ctx, store.UpdateSelection{All: true}); marked != 1 {
		t.Fatalf("mark all = %d, want 1", marked)
	}
}
