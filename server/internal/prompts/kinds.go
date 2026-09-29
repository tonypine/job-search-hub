package prompts

import "github.com/tonypine/job-search-hub/server/internal/store"

// KindInfo tells the owner what one agent's prompt is for and which
// placeholders the hub fills in it.
type KindInfo struct {
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Placeholders []string `json:"placeholders"`
}

// Kinds are the prompts the owner can edit, in the order a person meets them.
var Kinds = []KindInfo{
	{store.AgentRunKindCompanyTriage, "Company research", "Researches a company being added: its dossier, job board and people.",
		[]string{"{{company}}", "{{owner_profile}}", "{{company_dossier}}"}},
	{store.AgentRunKindJobFinder, "Find jobs", "Finds a company's open roles: sets its job board when the hub reads it, or records the roles off its careers page.",
		[]string{"{{company}}", "{{company_dossier}}", "{{job_criteria}}"}},
	{store.AgentPromptKindCompanySession, "Company session", "Starts a Claude session opened from a company.",
		[]string{"{{owner_profile}}", "{{owner_voice}}", "{{application_answers}}", "{{company_dossier}}"}},
	{store.AgentPromptKindJobSession, "Job session", "Starts a Claude session opened from a job.",
		[]string{"{{owner_profile}}", "{{owner_voice}}", "{{application_answers}}", "{{job_details}}", "{{company_dossier}}"}},
	{store.AgentPromptKindOutreachDraft, "Draft outreach", "The request Draft outreach types into a company's or job's session.", []string{}},
	{store.AgentPromptKindRecruiterReply, "Recruiter reply", "Drafts a message back to a recruiter who wrote before.",
		[]string{"{{owner_profile}}", "{{owner_voice}}", "{{recruiter_conversation}}", "{{openings}}"}},
	{store.AgentPromptKindProfileAudit, "LinkedIn profile audit", "Audits the LinkedIn profile for the recruiters searching.",
		[]string{"{{owner_profile}}", "{{criteria}}", "{{market}}", "{{recruiter_history}}"}},
	{store.AgentPromptKindJobFacts, "Job facts", "Tells the local model which facts to read from each posting; its schema lists them.", []string{}},
	{store.AgentPromptKindMailTriage, "Mail triage", "Tells the local model how to sort received mail no rule could sort.", []string{}},
	{store.AgentPromptKindLinkedInConversation, "LinkedIn conversations", "Tells the local model how to sort LinkedIn conversations others started.", []string{}},
}
