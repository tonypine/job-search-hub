// Package followupreminders tells the owner when a card's follow-up falls
// due, once per due date, as an update that reaches the apps and the paired
// phones, so a follow-up doesn't wait on the owner opening the board.
package followupreminders

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// remindHour is the local hour from which the day's due follow-ups are
	// told, so a reminder doesn't land in the night.
	remindHour = 8
	// Kind is the kind of update a reminder records.
	Kind = "follow_up_due"
)

type followUps interface {
	ListFollowUpsToRemind(ctx context.Context, dueBefore time.Time) ([]store.DueFollowUp, error)
	MarkFollowUpReminded(ctx context.Context, id uuid.UUID, dueAt time.Time) error
}

type updateRecorder interface {
	Record(ctx context.Context, input store.NewUpdate) (store.Update, error)
}

type Reminder struct {
	hub     followUps
	updates updateRecorder
	now     func() time.Time
}

func NewReminder(hub followUps, updates updateRecorder) *Reminder {
	return &Reminder{hub: hub, updates: updates, now: time.Now}
}

// Run reminds once at start, then every interval, until ctx ends.
func (reminder *Reminder) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			if reminded, err := reminder.RemindOnce(ctx); err != nil {
				slog.Error("follow-up reminders stopped", "error", err, "reminded", reminded)
			} else if reminded > 0 {
				slog.Info("follow-up reminders sent", "reminded", reminded)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RemindOnce records an update for each card whose follow-up is due today
// or earlier and hasn't been told, from remindHour on. It returns how many
// it told.
func (reminder *Reminder) RemindOnce(ctx context.Context) (int, error) {
	now := reminder.now()
	if now.Hour() < remindHour {
		return 0, nil
	}
	year, month, day := now.Date()
	dueBefore := time.Date(year, month, day+1, 0, 0, 0, 0, now.Location())
	due, err := reminder.hub.ListFollowUpsToRemind(ctx, dueBefore)
	if err != nil {
		return 0, err
	}
	for index, followUp := range due {
		if _, err := reminder.updates.Record(ctx, describe(followUp, now)); err != nil {
			return index, fmt.Errorf("remind of card %s: %w", followUp.ApplicationID, err)
		}
		if err := reminder.hub.MarkFollowUpReminded(ctx, followUp.ApplicationID, followUp.DueAt); err != nil {
			return index, err
		}
	}
	return len(due), nil
}

// describe words the reminder: what to follow up on, and since when the
// card has waited.
func describe(followUp store.DueFollowUp, now time.Time) store.NewUpdate {
	company := "the company"
	if followUp.CompanyName != nil && *followUp.CompanyName != "" {
		company = *followUp.CompanyName
	}
	title := "Follow up with " + company
	if followUp.JobTitle != nil && *followUp.JobTitle != "" {
		title = "Follow up on " + *followUp.JobTitle + " at " + company
	}
	body := []string{fmt.Sprintf("In %s since %s.", followUp.PhaseName, formatDay(followUp.PhaseEnteredAt, now))}
	if followUp.LastFollowedUpAt != nil && followUp.LastFollowedUpAt.After(followUp.PhaseEnteredAt) {
		body = append(body, fmt.Sprintf("Last followed up %s.", formatDay(*followUp.LastFollowedUpAt, now)))
	}
	if countDays(followUp.DueAt, now) > 0 {
		body = append(body, fmt.Sprintf("Due %s.", formatDay(followUp.DueAt, now)))
	}
	return store.NewUpdate{Kind: Kind, Title: title, Body: strings.Join(body, " "), JobID: followUp.JobID, CompanyID: followUp.CompanyID}
}

// formatDay names the calendar day of moment as seen at now.
func formatDay(moment, now time.Time) string {
	moment = moment.In(now.Location())
	if moment.Year() != now.Year() {
		return moment.Format("Jan 2, 2006")
	}
	return moment.Format("Jan 2")
}

// countDays is how many calendar days moment lies before now.
func countDays(moment, now time.Time) int {
	moment = moment.In(now.Location())
	from := time.Date(moment.Year(), moment.Month(), moment.Day(), 12, 0, 0, 0, now.Location())
	to := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	return int(to.Sub(from).Round(24*time.Hour) / (24 * time.Hour))
}
