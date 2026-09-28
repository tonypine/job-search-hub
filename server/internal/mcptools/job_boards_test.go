package mcptools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestSetJobBoardStoresAVerifiedBoardOnTheDossier(t *testing.T) {
	hub := startHub(t)
	session := connect(t, hub, ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	board := callTool[store.JobBoard](t, session, "set_job_board", map[string]any{
		"company_id": created.Company.ID, "provider": "greenhouse", "board_token": "acme", "source_url": "https://acme.com/careers",
	})
	if board.VerifiedAt == nil || board.OpenPostingCount == nil || *board.OpenPostingCount != 5 || board.BoardURL != "https://boards.example/greenhouse/acme" {
		t.Fatalf("board = %+v", board)
	}
	callTool[store.JobBoard](t, session, "set_job_board", map[string]any{
		"company_id": created.Company.ID, "provider": "greenhouse", "board_token": "acme",
	})

	dossier := callTool[store.CompanyDossier](t, session, "get_company", map[string]any{"company_id": created.Company.ID})
	if len(dossier.JobBoards) != 1 || dossier.JobBoards[0].ID != board.ID {
		t.Fatalf("dossier job boards = %+v, want the one board", dossier.JobBoards)
	}
	var boardChanges int
	err := hub.pool.QueryRow(context.Background(), `SELECT count(*) FROM changes WHERE entity_type = 'job_board'`).Scan(&boardChanges)
	if err != nil || boardChanges != 2 {
		t.Fatalf("job board changes = %d, want 2", boardChanges)
	}
}

func TestSetJobBoardRejectsABoardTheProviderDoesNotKnow(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	text := callFailingTool(t, session, "set_job_board", map[string]any{
		"company_id": created.Company.ID, "provider": "lever", "board_token": "wrong-guess",
	})
	if !strings.Contains(text, "wrong-guess") {
		t.Fatalf("error = %q", text)
	}
	if dossier := callTool[store.CompanyDossier](t, session, "get_company", map[string]any{"company_id": created.Company.ID}); len(dossier.JobBoards) != 0 {
		t.Fatalf("job boards = %+v, want none stored", dossier.JobBoards)
	}
}

func TestSetJobBoardStoresUnsupportedProvidersUnverified(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	board := callTool[store.JobBoard](t, session, "set_job_board", map[string]any{
		"company_id": created.Company.ID, "provider": "workable", "board_token": "acme", "board_url": "https://apply.workable.com/acme",
	})
	if board.VerifiedAt != nil || board.OpenPostingCount != nil || board.BoardURL != "https://apply.workable.com/acme" {
		t.Fatalf("board = %+v", board)
	}
}

func TestSetJobBoardStoresAVerifiedBoardWhosePostingsCannotBeCounted(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Acme", "domain": "acme.com"})

	board := callTool[store.JobBoard](t, session, "set_job_board", map[string]any{
		"company_id": created.Company.ID, "provider": "ashby", "board_token": "pageonly",
	})
	if board.VerifiedAt == nil || board.OpenPostingCount != nil {
		t.Fatalf("board = %+v, want verified with no count", board)
	}
}
