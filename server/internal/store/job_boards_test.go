package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestSetJobBoardStoresOneRowPerBoardAndRecordsEachWrite(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()
	company, _, err := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	firstCount, secondCount := 5, 7

	first, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{
		CompanyID: company.ID, Provider: "greenhouse", BoardToken: "acme", OpenPostingCount: &firstCount, SourceURL: "https://acme.com/careers",
	})
	if err != nil || first.VerifiedAt == nil || *first.OpenPostingCount != 5 {
		t.Fatalf("first set = %+v, err = %v", first, err)
	}
	second, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{
		CompanyID: company.ID, Provider: "greenhouse", BoardToken: "acme", OpenPostingCount: &secondCount,
	})
	if err != nil || second.ID != first.ID || *second.OpenPostingCount != 7 {
		t.Fatalf("second set = %+v, err = %v", second, err)
	}

	boards, err := hub.ListJobBoards(ctx, company.ID)
	if err != nil || len(boards) != 1 {
		t.Fatalf("boards = %+v, err = %v", boards, err)
	}
	rows, err := pool.Query(ctx, `SELECT operation FROM changes WHERE entity_type = 'job_board' ORDER BY id`)
	if err != nil {
		t.Fatalf("query changes: %v", err)
	}
	operations, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(operations) != 2 || operations[0] != "create" || operations[1] != "update" {
		t.Fatalf("job board changes = %v, want create then update", operations)
	}
}

func TestSetJobBoardStoresAnUnverifiedBoardWithoutACount(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})

	board, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{
		CompanyID: company.ID, Provider: "workable", BoardToken: "acme", BoardURL: "https://apply.workable.com/acme",
	})
	if err != nil || board.VerifiedAt != nil || board.OpenPostingCount != nil || board.BoardURL != "https://apply.workable.com/acme" {
		t.Fatalf("board = %+v, err = %v", board, err)
	}
}

func TestSetJobBoardRefusesAnotherCompanysBoardAndBadInput(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	acme, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	other, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Other", Domain: "other.com"})
	if _, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: acme.ID, Provider: "lever", BoardToken: "acme"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	if _, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: other.ID, Provider: "lever", BoardToken: "acme"}); !errors.Is(err, store.ErrJobBoardTaken) {
		t.Errorf("another company's board: err = %v, want ErrJobBoardTaken", err)
	}
	if _, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: acme.ID, Provider: "workday", BoardToken: "acme"}); err == nil {
		t.Error("expected an error for an unknown provider")
	}
	if _, err := hub.SetJobBoard(ctx, owner, store.JobBoardInput{CompanyID: uuid.New(), Provider: "ashby", BoardToken: "x"}); !errors.Is(err, store.ErrCompanyNotFound) {
		t.Errorf("unknown company: err = %v, want ErrCompanyNotFound", err)
	}
}
