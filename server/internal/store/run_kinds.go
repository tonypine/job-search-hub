package store

// RunKindJobBriefFull is a job's full brief, which Claude writes with the
// job_brief prompt; its runs are recorded apart from the routed briefs.
const RunKindJobBriefFull = "job_brief_full"

// The groups of run kinds, by what they work on.
const (
	RunGroupJobs      = "jobs"
	RunGroupCompanies = "companies"
	RunGroupMail      = "mail_and_messages"
	RunGroupYou       = "you"
	RunGroupSources   = "sources"
)

// RunGroup is a group of run kinds, as the owner names it.
type RunGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// RunGroups are the groups in the order they're shown.
var RunGroups = []RunGroup{
	{ID: RunGroupJobs, Title: "Jobs"},
	{ID: RunGroupCompanies, Title: "Companies"},
	{ID: RunGroupMail, Title: "Mail and messages"},
	{ID: RunGroupYou, Title: "You"},
	{ID: RunGroupSources, Title: "Sources"},
}

// The things a run is for. A run of a kind whose subject is
// RunSubjectNone is for nothing in particular, such as a pass over a feed.
const (
	RunSubjectJob                  = "job"
	RunSubjectCompany              = "company"
	RunSubjectMailMessage          = "mail_message"
	RunSubjectLinkedInConversation = "linkedin_conversation"
	RunSubjectProfile              = "profile"
	RunSubjectNone                 = "none"
)

// The ways a kind runs: one request to a routed model, one turn of the
// Claude CLI, or a Claude Code agent with tools.
const (
	RunEngineModelCall     = "model_call"
	RunEngineClaudeOneShot = "claude_one_shot"
	RunEngineClaudeAgent   = "claude_agent"
)

// RunKind is a kind of work the hub runs and records a run of.
type RunKind struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Group       string `json:"group"`
	SubjectType string `json:"subject_type"`
	Engine      string `json:"engine"`
	// Automatic says the hub starts it by itself; otherwise it runs only
	// when the owner asks.
	Automatic bool `json:"automatic"`
}

// RunKinds are every kind of run, in the order they're shown: by group, then
// by how often the owner looks at them.
var RunKinds = []RunKind{
	{ID: AgentPromptKindJobFacts, Title: "Job facts", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineModelCall, Automatic: true,
		Description: "Reads a job's seniority, pay, location and skills from its posting."},
	{ID: AgentPromptKindJobBrief, Title: "Job briefs", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineModelCall, Automatic: true,
		Description: "Writes a short brief of how well a job fits you."},
	{ID: RunKindJobBriefFull, Title: "Full briefs", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineClaudeOneShot, Automatic: true,
		Description: "Claude writes a full brief of the best-matched jobs each night, and of a job you ask about."},
	{ID: AgentPromptKindJobCV, Title: "Tailored CVs", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineClaudeOneShot, Automatic: true,
		Description: "Claude tailors your base CV to a job you pursue or that fits you well."},
	{ID: AgentPromptKindRecruiterScreen, Title: "Recruiter screens", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineModelCall, Automatic: true,
		Description: "Reads a pursued job's tailored CV as its recruiter would, listing the likely reasons to reject it."},
	{ID: AgentPromptKindInterviewPrep, Title: "Interview packs", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineModelCall, Automatic: true,
		Description: "Writes a pursued job's interview pack: the likely questions and the cases to tell."},
	{ID: AgentRunKindJobFix, Title: "Job fixes", Group: RunGroupJobs, SubjectType: RunSubjectJob, Engine: RunEngineClaudeAgent,
		Description: "A Claude agent corrects a job's details from your note."},
	{ID: AgentRunKindCompanyTriage, Title: "Company research", Group: RunGroupCompanies, SubjectType: RunSubjectCompany, Engine: RunEngineClaudeAgent,
		Description: "A Claude agent researches a company and fills in its dossier."},
	{ID: AgentRunKindJobFinder, Title: "Find jobs", Group: RunGroupCompanies, SubjectType: RunSubjectCompany, Engine: RunEngineClaudeAgent,
		Description: "A Claude agent finds a company's open roles."},
	{ID: AgentPromptKindMailTriage, Title: "Mail sorting", Group: RunGroupMail, SubjectType: RunSubjectMailMessage, Engine: RunEngineModelCall, Automatic: true,
		Description: "Sorts the mail no rule could sort."},
	{ID: AgentPromptKindLinkedInConversation, Title: "LinkedIn conversations", Group: RunGroupMail, SubjectType: RunSubjectLinkedInConversation,
		Engine: RunEngineModelCall, Automatic: true, Description: "Sorts your LinkedIn conversations."},
	{ID: AgentPromptKindMarketGaps, Title: "Market gaps", Group: RunGroupYou, SubjectType: RunSubjectNone, Engine: RunEngineModelCall, Automatic: true,
		Description: "Plans how to close the skills good fits keep asking for."},
	{ID: AgentRunKindProfileSeed, Title: "Knowledge base build", Group: RunGroupYou, SubjectType: RunSubjectProfile, Engine: RunEngineClaudeAgent,
		Description: "A Claude agent builds your knowledge base from your CV, LinkedIn export and answers."},
	{ID: AgentPromptKindHiringThread, Title: "Hacker News hiring thread", Group: RunGroupSources, SubjectType: RunSubjectNone, Engine: RunEngineModelCall,
		Automatic: true, Description: "Reads the roles in the comments of Hacker News' monthly hiring thread."},
}

// GetRunKind returns the kind with id, and whether there is one.
func GetRunKind(id string) (RunKind, bool) {
	for _, kind := range RunKinds {
		if kind.ID == id {
			return kind, true
		}
	}
	return RunKind{}, false
}

// getRunSubjectType is the subject type a run of kind is stored with, nil
// when its kind has no subject or isn't in the catalog.
func getRunSubjectType(kind string) *string {
	runKind, found := GetRunKind(kind)
	if !found || runKind.SubjectType == RunSubjectNone {
		return nil
	}
	return &runKind.SubjectType
}
