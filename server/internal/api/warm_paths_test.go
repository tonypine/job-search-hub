package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestSomeoneTheOwnerKnowsIsAWarmPathAtEachCompanyTheyCanHelpWith(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	acme, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	globex, _, _ := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Globex", Domain: "globex.com"})

	status, body := send(t, http.MethodPost, service.url+"/v1/companies/"+acme.ID.String()+"/warm-paths", ownerToken,
		`{"name":"Sam","how_known":"Former colleague","preferred_channel":"LinkedIn","note":"Interviewed there"}`)
	var added store.WarmPath
	if err := json.Unmarshal(body, &added); status != http.StatusCreated || err != nil || added.Name != "Sam" {
		t.Fatalf("add: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/companies/"+globex.ID.String()+"/warm-paths", ownerToken,
		`{"name":"sam ","note":"Knows the CTO"}`); status != http.StatusCreated {
		t.Fatalf("the same person at another company: %d %s", status, body)
	}

	dossier, err := service.hub.GetCompanyDossier(ctx, globex.ID)
	if err != nil || len(dossier.WarmPaths) != 1 {
		t.Fatalf("globex's warm paths: %+v, %v", dossier.WarmPaths, err)
	}
	if path := dossier.WarmPaths[0]; path.ContactID != added.ContactID || path.HowKnown != "Former colleague" || path.PreferredChannel != "LinkedIn" || path.Note != "Knows the CTO" {
		t.Fatalf("globex's warm path = %+v; want Sam, as known, with Globex's note", path)
	}

	if status, _ := send(t, http.MethodDelete, service.url+"/v1/companies/"+acme.ID.String()+"/warm-paths/"+added.ContactID.String(), ownerToken, ""); status != http.StatusNoContent {
		t.Fatalf("remove: %d", status)
	}
	if dossier, _ := service.hub.GetCompanyDossier(ctx, acme.ID); len(dossier.WarmPaths) != 0 {
		t.Errorf("acme still lists %+v", dossier.WarmPaths)
	}
	if dossier, _ := service.hub.GetCompanyDossier(ctx, globex.ID); len(dossier.WarmPaths) != 1 {
		t.Errorf("removing Sam from Acme removed them from Globex")
	}
	agentToken := startTriage(t, service).Token
	if status, _ := send(t, http.MethodPost, service.url+"/v1/companies/"+acme.ID.String()+"/warm-paths", agentToken, `{"name":"Someone"}`); status != http.StatusForbidden {
		t.Errorf("an agent's add: %d, want 403", status)
	}
}
