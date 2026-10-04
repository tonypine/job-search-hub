package prompts_test

import (
	"slices"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/prompts"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestEveryPromptKindIsOneTheOwnerCanEdit(t *testing.T) {
	var editable []string
	for _, info := range prompts.Kinds {
		editable = append(editable, info.Kind)
	}
	if kinds := slices.Sorted(slices.Values(store.AgentPromptKinds)); !slices.Equal(slices.Sorted(slices.Values(editable)), kinds) {
		t.Fatalf("editable kinds = %v, want %v", editable, kinds)
	}
}
