// Package companytriage holds the contract of the agent that fills a company's
// dossier: the shape of the result it must answer with. Its instructions are
// an editable prompt stored in the hub, not code.
package companytriage

import _ "embed"

//go:embed result.schema.json
var ResultSchema string

// Result is the triage agent's answer, as ResultSchema describes it.
type Result struct {
	CompanyID   *string   `json:"company_id"`
	Domain      string    `json:"domain"`
	JobBoard    *JobBoard `json:"job_board"`
	PeopleAdded int       `json:"people_added"`
	Unresolved  []string  `json:"unresolved"`
	Summary     string    `json:"summary"`
}

type JobBoard struct {
	Provider   string `json:"provider"`
	BoardToken string `json:"board_token"`
	Verified   bool   `json:"verified"`
}
