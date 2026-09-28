package mcptools_test

import (
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type watchList struct {
	Companies []store.WatchedCompany `json:"companies"`
}

type addedToWatchList struct {
	WatchedSince time.Time `json:"watched_since"`
	Added        bool      `json:"added"`
}

type removedFromWatchList struct {
	Removed bool `json:"removed"`
}

func TestTheWatchListOverMCP(t *testing.T) {
	session := connect(t, startHub(t), ownerToken)
	created := callTool[createdCompany](t, session, "create_company", map[string]any{"name": "Stripe", "domain": "stripe.com"})
	companyID := map[string]any{"company_id": created.Company.ID}

	if empty := callTool[watchList](t, session, "list_watch_list", map[string]any{}); len(empty.Companies) != 0 {
		t.Fatalf("initial watch list = %+v, want empty", empty)
	}

	added := callTool[addedToWatchList](t, session, "add_to_watch_list", companyID)
	if !added.Added {
		t.Fatalf("add = %+v", added)
	}
	if again := callTool[addedToWatchList](t, session, "add_to_watch_list", companyID); again.Added {
		t.Fatalf("second add = %+v, want added=false", again)
	}

	listed := callTool[watchList](t, session, "list_watch_list", map[string]any{})
	if len(listed.Companies) != 1 || listed.Companies[0].Company.ID != created.Company.ID {
		t.Fatalf("watch list = %+v", listed)
	}
	if dossier := callTool[companyDossier](t, session, "get_company", companyID); dossier.WatchedSince == nil {
		t.Fatal("get_company does not report the company as watched")
	}

	if removed := callTool[removedFromWatchList](t, session, "remove_from_watch_list", companyID); !removed.Removed {
		t.Fatalf("remove = %+v", removed)
	}
	if after := callTool[watchList](t, session, "list_watch_list", map[string]any{}); len(after.Companies) != 0 {
		t.Fatalf("watch list after remove = %+v", after)
	}
	if dossier := callTool[companyDossier](t, session, "get_company", companyID); dossier.WatchedSince != nil {
		t.Fatalf("get_company still reports watched since %v", dossier.WatchedSince)
	}
}
