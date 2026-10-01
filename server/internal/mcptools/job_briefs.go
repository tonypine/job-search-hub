package mcptools

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type fullBriefWriter interface {
	WriteFullBrief(ctx context.Context, jobID uuid.UUID) error
}

type briefReader interface {
	GetJobBrief(ctx context.Context, jobID uuid.UUID) (*store.JobBrief, error)
}

type writeFullBriefInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the job to brief"`
}

type writeFullBriefOutput struct {
	Match  string `json:"match"`
	Reason string `json:"reason"`
}

// AddJobBriefTools gives the owner's server write_full_brief. Agents don't
// get it: it spends the owner's Claude plan.
func AddJobBriefTools(server *mcp.Server, writer fullBriefWriter, briefs briefReader) {
	addTool(server, &mcp.Tool{
		Name: "write_full_brief",
		Description: "Have Claude write a job's full brief now: its match, the reason, and the strengths and weaknesses citing the knowledge base. " +
			"Returns when it's saved; the job's details show it over the local pre-brief. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input writeFullBriefInput) (*mcp.CallToolResult, writeFullBriefOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, writeFullBriefOutput{}, err
		}
		if err := writer.WriteFullBrief(ctx, input.JobID); err != nil {
			return nil, writeFullBriefOutput{}, err
		}
		brief, err := briefs.GetJobBrief(ctx, input.JobID)
		if err != nil || brief == nil {
			return nil, writeFullBriefOutput{}, err
		}
		return nil, writeFullBriefOutput{Match: brief.Match, Reason: brief.Reason}, nil
	})
}

type cvDrafter interface {
	DraftCV(ctx context.Context, jobID uuid.UUID) (store.CV, error)
	GenerateCV(ctx context.Context, jobID uuid.UUID) (store.CV, error)
	GenerateMissingCVs(ctx context.Context) (int, error)
}

type generateCVInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the job whose CV to generate"`
}

type generateCVOutput struct {
	CVID uuid.UUID `json:"cv_id"`
	// PDFPath is the file to open or attach to a form; empty when the hub
	// can't print.
	PDFPath string `json:"pdf_path,omitempty"`
}

type generateMissingCVsOutput struct {
	Queued int `json:"queued"`
}

type draftCVInput struct {
	JobID uuid.UUID `json:"job_id" jsonschema:"the pursued job to tailor the CV to"`
}

type draftCVOutput struct {
	CVID     uuid.UUID `json:"cv_id"`
	Headline string    `json:"headline"`
	Summary  string    `json:"summary"`
}

// AddCVTools gives the owner's server draft_cv. Agents don't get it: it
// spends the owner's Claude plan.
func AddCVTools(server *mcp.Server, drafter cvDrafter) {
	addTool(server, &mcp.Tool{
		Name: "generate_cv",
		Description: "Make sure a job has its tailored CV as a PDF, and return the file's path, to open or to attach to an application " +
			"form. Drafts the CV with Claude when the job has none (this takes a minute), and prints it when it isn't printed. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input generateCVInput) (*mcp.CallToolResult, generateCVOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, generateCVOutput{}, err
		}
		cv, err := drafter.GenerateCV(ctx, input.JobID)
		if err != nil {
			return nil, generateCVOutput{}, err
		}
		return nil, generateCVOutput{CVID: cv.ID, PDFPath: cv.PDFPath}, nil
	})
	addTool(server, &mcp.Tool{
		Name: "generate_missing_cvs",
		Description: "Start generating the tailored CV PDFs of every good-fit or pursued job that has none yet, in the background, " +
			"and return how many it will make; 0 when none are missing or a run is already going. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, generateMissingCVsOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, generateMissingCVsOutput{}, err
		}
		queued, err := drafter.GenerateMissingCVs(ctx)
		return nil, generateMissingCVsOutput{Queued: queued}, err
	})
	addTool(server, &mcp.Tool{
		Name: "draft_cv",
		Description: "Have Claude tailor the base CV to a job now: a new headline, summary and bullets, each bullet citing a base CV bullet " +
			"or a confirmed knowledge-base entry. Replaces the job's earlier draft. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input draftCVInput) (*mcp.CallToolResult, draftCVOutput, error) {
		if _, err := getOwnerActor(ctx); err != nil {
			return nil, draftCVOutput{}, err
		}
		cv, err := drafter.DraftCV(ctx, input.JobID)
		if err != nil {
			return nil, draftCVOutput{}, err
		}
		return nil, draftCVOutput{CVID: cv.ID, Headline: cv.Content.Basics.Label, Summary: cv.Content.Basics.Summary}, nil
	})
}
