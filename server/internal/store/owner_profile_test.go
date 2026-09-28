package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

func TestTheOwnerProfileStartsEmptyAndRecordsEachSave(t *testing.T) {
	pool := testdatabase.New(t)
	hub := store.New(pool)
	ctx := context.Background()

	empty, err := hub.GetOwnerProfile(ctx)
	if err != nil || empty.Body != "" {
		t.Fatalf("initial profile = %+v, err = %v", empty, err)
	}

	for _, body := range []string{"# Candidate\nSenior engineer.", "# Candidate\nSenior engineer, remote."} {
		saved, err := hub.SaveOwnerProfile(ctx, owner, body)
		if err != nil || saved.Body != body {
			t.Fatalf("save %q: %+v, %v", body, saved, err)
		}
	}
	if _, err := hub.SaveOwnerProfile(ctx, owner, "# Candidate\nSenior engineer, remote."); err != nil {
		t.Fatalf("unchanged save: %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT before, after FROM changes WHERE entity_type = 'owner_profile' ORDER BY id`)
	if err != nil {
		t.Fatalf("query changes: %v", err)
	}
	defer rows.Close()
	var bodies [][2]string
	for rows.Next() {
		var before, after []byte
		if err := rows.Scan(&before, &after); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var beforeBody, afterBody map[string]string
		if err := json.Unmarshal(before, &beforeBody); err != nil {
			t.Fatalf("decode before: %v", err)
		}
		if err := json.Unmarshal(after, &afterBody); err != nil {
			t.Fatalf("decode after: %v", err)
		}
		bodies = append(bodies, [2]string{beforeBody["body"], afterBody["body"]})
	}
	if len(bodies) != 2 || bodies[0][0] != "" || bodies[1][0] != "# Candidate\nSenior engineer." || bodies[1][1] != "# Candidate\nSenior engineer, remote." {
		t.Fatalf("recorded before/after = %q, want two saves and nothing for the unchanged one", bodies)
	}
}
