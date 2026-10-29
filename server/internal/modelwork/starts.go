package modelwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

// startedRunTimeout bounds a run the owner started, its wait for its turn
// included.
const startedRunTimeout = 10 * time.Minute

// ErrNotStartable means the kind isn't one the owner starts for a job or a
// company.
var ErrNotStartable = errors.New("that kind of run isn't started for a job or a company")

// CannotRunError says why a kind can't run for its subject yet, such as
// "needs the tailored CV first". One that is off is ErrNotSetUp too.
type CannotRunError struct {
	Reason string
	off    bool
}

func (err CannotRunError) Error() string { return err.Reason }

func (err CannotRunError) Unwrap() error {
	if err.off {
		return ErrNotSetUp
	}
	return nil
}

// InvalidStartError says what a start asked for that can't be, such as a
// provider with no model.
type InvalidStartError struct{ Reason string }

func (err InvalidStartError) Error() string { return err.Reason }

// notRoutedReason is why a kind with no model routed for it can't run.
const notRoutedReason = "no model is routed for it yet"

type jobBriefWriter interface {
	WriteBrief(ctx context.Context, jobID uuid.UUID) error
}

type fullBriefWriter interface {
	WriteFullBrief(ctx context.Context, jobID uuid.UUID) error
}

type cvDrafter interface {
	DraftCV(ctx context.Context, jobID uuid.UUID) (store.CV, error)
}

type cvScreener interface {
	ScreenJob(ctx context.Context, jobID uuid.UUID) error
}

type packPreparer interface {
	PrepareJob(ctx context.Context, jobID uuid.UUID) error
}

type updateRecorder interface {
	Record(ctx context.Context, input store.NewUpdate) (store.Update, error)
}

// subject is the job or company a run is started for, as its kinds'
// checks read it.
type subject struct {
	job     *store.JobDetails
	company *store.Company
	// hasBaseCV says the owner saved a base CV, which tailored CVs need.
	hasBaseCV bool
}

// title names the subject in an update.
func (subject subject) title() string {
	if subject.company != nil {
		return subject.company.Name
	}
	if subject.job.CompanyName != nil {
		return subject.job.Job.Title + " at " + *subject.job.CompanyName
	}
	return subject.job.Job.Title
}

// starter is how the owner starts a kind for its subject.
type starter struct {
	// isOff says the hub has no worker for the kind, as when no model was
	// routed for it when the hub started.
	isOff func(controls *Controls) bool
	// check says why the kind can't run for the subject yet, "" when it can.
	check func(subject subject) string
	// run does the run. It is nil for an agent kind, which the Mac runs.
	run func(ctx context.Context, controls *Controls, subjectID uuid.UUID) error
	// noun names one run in an update: "full brief", "company research".
	noun string
	// done is what an update says of a run that ended well: "written".
	done string
}

func canAlwaysRun(subject) string { return "" }

// starters are every kind the owner can start for a job or a company. A
// kind in the catalog with either subject must have one.
var starters = map[string]starter{
	store.AgentPromptKindJobFacts: {
		isOff: func(controls *Controls) bool { return controls.Facts == nil },
		check: func(subject subject) string {
			if strings.TrimSpace(subject.job.Job.Description) == "" {
				return "needs the posting's text first"
			}
			return ""
		},
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			return controls.Facts.ReadJobNow(ctx, jobID)
		},
		noun: "job facts", done: "read",
	},
	store.AgentPromptKindJobBrief: {
		isOff: func(controls *Controls) bool { return controls.Briefs == nil },
		check: func(subject subject) string {
			if len(subject.job.RawFacts) == 0 {
				return "needs the job's facts first"
			}
			return ""
		},
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			return controls.Briefs.WriteBrief(ctx, jobID)
		},
		noun: "brief", done: "written",
	},
	store.RunKindJobBriefFull: {
		isOff: func(controls *Controls) bool { return controls.FullBriefs == nil },
		check: canAlwaysRun,
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			return controls.FullBriefs.WriteFullBrief(ctx, jobID)
		},
		noun: "full brief", done: "written",
	},
	store.AgentPromptKindJobCV: {
		isOff: func(controls *Controls) bool { return controls.CVs == nil },
		check: func(subject subject) string {
			if !subject.hasBaseCV {
				return "needs your base CV first"
			}
			return ""
		},
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			_, err := controls.CVs.DraftCV(ctx, jobID)
			return err
		},
		noun: "tailored CV", done: "drafted",
	},
	store.AgentPromptKindRecruiterScreen: {
		isOff: func(controls *Controls) bool { return controls.Screens == nil },
		check: func(subject subject) string {
			if subject.job.CVID == nil {
				return "needs the tailored CV first"
			}
			return ""
		},
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			return controls.Screens.ScreenJob(ctx, jobID)
		},
		noun: "recruiter screen", done: "done",
	},
	store.AgentPromptKindInterviewPrep: {
		isOff: func(controls *Controls) bool { return controls.Packs == nil },
		check: func(subject subject) string {
			if subject.job.Decision == nil || subject.job.Decision.Decision != store.JobDecisionPursue {
				return "once you pursue the job"
			}
			return ""
		},
		run: func(ctx context.Context, controls *Controls, jobID uuid.UUID) error {
			return controls.Packs.PrepareJob(ctx, jobID)
		},
		noun: "interview pack", done: "prepared",
	},
	store.AgentRunKindJobFix:        {isOff: isNeverOff, check: canAlwaysRun, noun: "a fix of the job's details"},
	store.AgentRunKindCompanyTriage: {isOff: isNeverOff, check: canAlwaysRun, noun: "company research"},
	store.AgentRunKindJobFinder:     {isOff: isNeverOff, check: canAlwaysRun, noun: "finding jobs"},
}

func isNeverOff(*Controls) bool { return false }

// IsStartable says whether the owner can start kind for its subject.
func IsStartable(kind string) bool {
	_, found := starters[kind]
	return found
}

// KindRoute is the provider and model a kind runs on.
type KindRoute struct {
	ProviderID uuid.UUID `json:"provider_id"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
}

// RunnableKind is a kind the owner can start for a job or a company, with
// its model, its latest run for the subject, and whether it can run now.
type RunnableKind struct {
	store.RunKind
	// Paused says the kind's own runs wait; one the owner starts doesn't.
	Paused bool `json:"paused"`
	// Route is what the kind runs on, nil when nothing is routed for it. An
	// agent kind with none runs on the model Claude Code picks.
	Route *KindRoute `json:"route,omitempty"`
	// LastRun is the kind's latest run for the subject, nil before its first.
	LastRun *store.Run `json:"last_run,omitempty"`
	CanRun  bool       `json:"can_run"`
	// Reason says why it can't run yet, when it can't.
	Reason string `json:"reason,omitempty"`
	// NeedsNote says a start takes a note: the owner's word on what to fix.
	NeedsNote bool `json:"needs_note"`
}

// ListRunnableKinds is every kind the owner can start for the job or the
// company, in the catalog's order. A subject that isn't there is
// store.ErrJobNotFound or store.ErrCompanyNotFound.
func (controls *Controls) ListRunnableKinds(ctx context.Context, subjectType string, subjectID uuid.UUID) ([]RunnableKind, error) {
	subject, err := controls.readSubject(ctx, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	kinds := []RunnableKind{}
	for _, kind := range store.RunKinds {
		starter, startable := starters[kind.ID]
		if !startable || kind.SubjectType != subjectType {
			continue
		}
		route, err := controls.readRoute(ctx, kind.ID)
		if err != nil {
			return nil, err
		}
		page, err := controls.Hub.ListRuns(ctx, store.RunFilter{Kind: kind.ID, SubjectType: subjectType, SubjectID: &subjectID, Limit: 1})
		if err != nil {
			return nil, err
		}
		runnable := RunnableKind{
			RunKind: kind, Paused: kind.Automatic && controls.Queue.IsPaused(kind.ID), Route: route,
			NeedsNote: kind.ID == store.AgentRunKindJobFix,
		}
		if len(page.Runs) > 0 {
			runnable.LastRun = &page.Runs[0]
		}
		if reason := getCannotRunReason(controls, kind, starter, subject, route != nil); reason != nil {
			runnable.Reason = reason.Reason
		} else {
			runnable.CanRun = true
		}
		kinds = append(kinds, runnable)
	}
	return kinds, nil
}

// getCannotRunReason says why the kind can't run for the subject now, nil
// when it can. A kind that runs on a model needs one: its route's, or one
// chosen for the run.
func getCannotRunReason(controls *Controls, kind store.RunKind, starter starter, subject subject, hasModel bool) *CannotRunError {
	if starter.isOff(controls) || (kind.Engine != store.RunEngineClaudeAgent && !hasModel) {
		return &CannotRunError{Reason: notRoutedReason, off: true}
	}
	if reason := starter.check(subject); reason != "" {
		return &CannotRunError{Reason: reason}
	}
	return nil
}

func (controls *Controls) readSubject(ctx context.Context, subjectType string, subjectID uuid.UUID) (subject, error) {
	if subjectType == store.RunSubjectCompany {
		company, err := controls.Hub.GetCompany(ctx, subjectID)
		return subject{company: &company}, err
	}
	details, err := controls.Hub.GetJobDetails(ctx, subjectID)
	if err != nil {
		return subject{}, err
	}
	_, err = controls.Hub.GetBaseCV(ctx)
	if errors.Is(err, store.ErrCVNotFound) {
		return subject{job: &details}, nil
	}
	return subject{job: &details, hasBaseCV: err == nil}, err
}

// readRoute is the kind's route with its provider's name, nil when it has
// none.
func (controls *Controls) readRoute(ctx context.Context, kind string) (*KindRoute, error) {
	route, err := controls.Hub.GetTaskRoute(ctx, kind)
	if errors.Is(err, store.ErrTaskRouteNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	provider, err := controls.Hub.GetModelProvider(ctx, route.ProviderID)
	if err != nil {
		return nil, err
	}
	return &KindRoute{ProviderID: provider.ID, Provider: provider.Name, Model: route.Model}, nil
}

// StartRequest starts a kind's run for a job or a company. A provider and
// model, or a model on the route's provider, are for this run only. A job
// fix needs its note.
type StartRequest struct {
	Kind string `json:"kind"`
	// SubjectType, when set, must be the kind's.
	SubjectType string     `json:"subject_type,omitempty"`
	SubjectID   uuid.UUID  `json:"subject_id"`
	ProviderID  *uuid.UUID `json:"provider_id,omitempty"`
	Model       string     `json:"model,omitempty"`
	Note        string     `json:"note,omitempty"`
}

// StartedRun is a run the owner started, as its start answers: the id it is
// recorded under once it runs, and what it runs on.
type StartedRun struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	SubjectType string    `json:"subject_type"`
	SubjectID   uuid.UUID `json:"subject_id"`
	// Provider and Model are the ones chosen for the run, else its route's;
	// both are empty for an agent run that leaves the model to Claude Code.
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// ChosenModel says the model was chosen for this run only.
	ChosenModel    bool `json:"chosen_model"`
	StartedByOwner bool `json:"started_by_owner"`
	// TaskID is the Mac's task an agent run waits in until the Mac runs it.
	TaskID *uuid.UUID `json:"task_id,omitempty"`
}

// StartRun starts the kind's run for the subject now, ahead of background
// work and whatever the kind's pause, and returns once it is queued. A model
// run's records and the job change as it ends; an agent run waits for the
// Mac. A kind that can't run yet is a CannotRunError and starts nothing.
func (controls *Controls) StartRun(ctx context.Context, actor store.Actor, request StartRequest) (StartedRun, error) {
	kind, found := store.GetRunKind(request.Kind)
	starter, startable := starters[request.Kind]
	if !found || !startable {
		return StartedRun{}, ErrNotStartable
	}
	if request.SubjectType != "" && request.SubjectType != kind.SubjectType {
		return StartedRun{}, InvalidStartError{Reason: fmt.Sprintf("%s run for a %s", kind.Title, kind.SubjectType)}
	}
	note := strings.TrimSpace(request.Note)
	if kind.ID == store.AgentRunKindJobFix && note == "" {
		return StartedRun{}, InvalidStartError{Reason: "a job fix needs a note saying what's wrong"}
	}
	subject, err := controls.readSubject(ctx, kind.SubjectType, request.SubjectID)
	if err != nil {
		return StartedRun{}, err
	}
	route, err := controls.readRoute(ctx, kind.ID)
	if err != nil {
		return StartedRun{}, err
	}
	chosen, err := controls.readChosenModel(ctx, kind, route, request)
	if err != nil {
		return StartedRun{}, err
	}
	if reason := getCannotRunReason(controls, kind, starter, subject, route != nil || chosen != nil); reason != nil {
		return StartedRun{}, *reason
	}

	started := StartedRun{ID: uuid.New(), Kind: kind.ID, SubjectType: kind.SubjectType, SubjectID: request.SubjectID, StartedByOwner: true}
	switch {
	case chosen != nil:
		started.Provider, started.Model, started.ChosenModel = chosen.Provider, chosen.Model, true
	case route != nil:
		started.Provider, started.Model = route.Provider, route.Model
	}
	if starter.run == nil {
		return controls.queueAgentRun(ctx, actor, starter, subject, started, chosen, note)
	}

	runCtx := chatcompletions.WithRunID(context.WithoutCancel(ctx), kind.ID, started.ID)
	runCtx = chatcompletions.WithStartedByOwner(modelqueue.WithPriority(runCtx, modelqueue.PriorityDispatched))
	if chosen != nil {
		runCtx = modelrouter.WithModel(runCtx, kind.ID, chosen.ProviderID, chosen.Model)
	}
	go func() {
		runCtx, cancel := context.WithTimeout(runCtx, startedRunTimeout)
		defer cancel()
		err := starter.run(runCtx, controls, request.SubjectID)
		if err != nil {
			slog.Warn("a started run failed", "kind", kind.ID, "subject", request.SubjectID, "run", started.ID, "error", err)
		}
		controls.recordRunEnded(context.WithoutCancel(ctx), kind, starter, subject, request.SubjectID, err)
	}()
	return started, nil
}

// readChosenModel is the provider and model the start chose for this run,
// nil when it chose none. A model alone runs on the route's provider. An
// agent kind runs only on Claude.
func (controls *Controls) readChosenModel(ctx context.Context, kind store.RunKind, route *KindRoute, request StartRequest) (*KindRoute, error) {
	model := strings.TrimSpace(request.Model)
	if request.ProviderID == nil && model == "" {
		return nil, nil
	}
	if model == "" {
		return nil, InvalidStartError{Reason: "a provider chosen for one run needs its model"}
	}
	if request.ProviderID == nil {
		if route == nil {
			return nil, InvalidStartError{Reason: kind.Title + " has no route: name the model's provider"}
		}
		return &KindRoute{ProviderID: route.ProviderID, Provider: route.Provider, Model: model}, nil
	}
	provider, err := controls.Hub.GetModelProvider(ctx, *request.ProviderID)
	if errors.Is(err, store.ErrModelProviderNotFound) {
		return nil, InvalidStartError{Reason: "no such provider"}
	}
	if err != nil {
		return nil, err
	}
	if kind.Engine == store.RunEngineClaudeAgent && provider.Kind != store.ModelProviderKindClaudeSubscription {
		return nil, InvalidStartError{Reason: kind.Title + " runs only on Claude"}
	}
	return &KindRoute{ProviderID: provider.ID, Provider: provider.Name, Model: model}, nil
}

// queueAgentRun asks the Mac to run an agent kind, under the started run's
// id and on the model chosen for it, and says so in an update.
func (controls *Controls) queueAgentRun(ctx context.Context, actor store.Actor, starter starter, subject subject, started StartedRun,
	chosen *KindRoute, note string) (StartedRun, error) {
	input := store.NewAgentRunTask{Kind: started.Kind, SubjectID: started.SubjectID, Note: note, RunID: started.ID}
	if chosen != nil {
		input.Model = chosen.Model
	}
	task, err := controls.Hub.QueueAgentRunTask(ctx, actor, input, nil)
	if err != nil {
		return StartedRun{}, err
	}
	started.TaskID = &task.ID
	if controls.Updates != nil {
		update := store.NewUpdate{Kind: "task_queued", Title: "You started " + starter.noun + " for " + subject.title(),
			Body: "Waiting for the Mac to pick it up.", CompanyID: task.CompanyID, JobID: task.JobID}
		if _, err := controls.Updates.Record(ctx, update); err != nil {
			slog.Warn("record a started run's update", "kind", started.Kind, "error", err)
		}
	}
	return started, nil
}

// recordRunEnded tells the apps a started run ended, so the job's page and
// Activity read it again.
func (controls *Controls) recordRunEnded(ctx context.Context, kind store.RunKind, starter starter, subject subject, subjectID uuid.UUID, err error) {
	if controls.Updates == nil {
		return
	}
	noun := strings.ToUpper(starter.noun[:1]) + starter.noun[1:]
	update := store.NewUpdate{Kind: RunFinishedUpdate, Title: noun + " " + starter.done + " for " + subject.title()}
	if err != nil {
		update.Title, update.Body = noun+" failed for "+subject.title(), err.Error()
	}
	if subject.job != nil {
		update.JobID, update.CompanyID = &subjectID, subject.job.Job.CompanyID
	} else {
		update.CompanyID = &subjectID
	}
	if _, err := controls.Updates.Record(ctx, update); err != nil {
		slog.Warn("record a started run's update", "kind", kind.ID, "error", err)
	}
}

// RunFinishedUpdate is the kind of update a run the owner started records
// when it ends. The owner is watching it, so it isn't pushed.
const RunFinishedUpdate = "run_finished"
