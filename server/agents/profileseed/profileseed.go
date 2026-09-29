// Package profileseed holds the contract of the agent that builds the owner's
// knowledge base from the CV, the LinkedIn export and the answers library:
// the shape of the result it answers with. Its instructions are an editable
// prompt stored in the hub, not code.
package profileseed

import _ "embed"

//go:embed result.schema.json
var ResultSchema string

// Result is the seeding agent's answer, as ResultSchema describes it.
type Result struct {
	EntriesAdded   int      `json:"entries_added"`
	EntriesUpdated int      `json:"entries_updated"`
	Conflicts      []string `json:"conflicts"`
	Summary        string   `json:"summary"`
}
