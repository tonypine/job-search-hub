package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type listApplicationAnswersOutput struct {
	Answers []store.ApplicationAnswer `json:"answers"`
}

type saveApplicationAnswerInput struct {
	Question string `json:"question" jsonschema:"the question as application forms ask it"`
	Answer   string `json:"answer" jsonschema:"the candidate's answer, as they gave it"`
}

func addApplicationAnswerTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name: "list_application_answers",
		Description: "List the candidate's saved answers to application form questions: notice period, salary expectation, work " +
			"authorization and the like. Answer a form from these, the same way every time; an empty answer is a question still to ask them.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listApplicationAnswersOutput, error) {
		answers, err := hub.ListApplicationAnswers(ctx)
		return nil, listApplicationAnswersOutput{Answers: answers}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "save_application_answer",
		Description: "Save the candidate's answer to an application form question, replacing their answer to the same question. Only with their say-so. Owner only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveApplicationAnswerInput) (*mcp.CallToolResult, store.ApplicationAnswer, error) {
		actor, err := getOwnerActor(ctx)
		if err != nil {
			return nil, store.ApplicationAnswer{}, err
		}
		saved, err := hub.SaveApplicationAnswer(ctx, actor, nil, input.Question, input.Answer)
		return nil, saved, err
	})
}
