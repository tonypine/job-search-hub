// Package jobfinder holds the contract of the agent that finds a company's
// open roles: the shape of the result it answers with. Its instructions are
// an editable prompt stored in the hub, not code.
package jobfinder

import _ "embed"

//go:embed result.schema.json
var ResultSchema string

// Result is the job finder's answer, as ResultSchema describes it.
type Result struct {
	CompanyID    string    `json:"company_id"`
	CareersURL   *string   `json:"careers_url"`
	JobBoard     *JobBoard `json:"job_board"`
	JobsRecorded int       `json:"jobs_recorded"`
	Unresolved   []string  `json:"unresolved"`
	Summary      string    `json:"summary"`
}

type JobBoard struct {
	Provider   string `json:"provider"`
	BoardToken string `json:"board_token"`
	Verified   bool   `json:"verified"`
}
