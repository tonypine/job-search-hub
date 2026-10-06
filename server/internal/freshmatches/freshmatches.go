// Package freshmatches tells the owner when a job judged a strong match is
// still in its first few days, once per job, as an update that reaches the
// apps and the paired phones, so a good posting isn't found after the
// earlier applicants have filled its pipeline.
package freshmatches

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
	// FreshFor is how long after it's posted a job is still worth telling:
	// response odds drop once a posting is a few days old.
	FreshFor = 3 * 24 * time.Hour
	// quietFromHour and quietUntilHour bound the local night, when matches
	// wait to be told in the morning.
	quietFromHour  = 22
	quietUntilHour = 8
	// Kind is the kind of update a fresh match records.
	Kind = "fresh_match"
)

type freshMatches interface {
	ListFreshMatchesToTell(ctx context.Context, postedAfter time.Time) ([]store.FreshMatch, error)
	MarkFreshMatchTold(ctx context.Context, jobID uuid.UUID, toldAt time.Time) error
}

type updateRecorder interface {
	Record(ctx context.Context, input store.NewUpdate) (store.Update, error)
}

type Teller struct {
	hub     freshMatches
	updates updateRecorder
	now     func() time.Time
}

func NewTeller(hub freshMatches, updates updateRecorder) *Teller {
	return &Teller{hub: hub, updates: updates, now: time.Now}
}

// Run tells once at start, then every interval, until ctx ends.
func (teller *Teller) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !drain.IsDraining(ctx) {
			if told, err := teller.TellOnce(ctx); err != nil {
				slog.Error("fresh matches stopped", "error", err, "told", told)
			} else if told > 0 {
				slog.Info("fresh matches told", "told", told)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// TellOnce records an update for each fresh strong match not yet told,
// outside the night. It returns how many it told.
func (teller *Teller) TellOnce(ctx context.Context) (int, error) {
	now := teller.now()
	if now.Hour() >= quietFromHour || now.Hour() < quietUntilHour {
		return 0, nil
	}
	matches, err := teller.hub.ListFreshMatchesToTell(ctx, now.Add(-FreshFor))
	if err != nil {
		return 0, err
	}
	for index, match := range matches {
		if _, err := teller.updates.Record(ctx, describe(match, now)); err != nil {
			return index, fmt.Errorf("tell of job %s: %w", match.JobID, err)
		}
		if err := teller.hub.MarkFreshMatchTold(ctx, match.JobID, now); err != nil {
			return index, err
		}
	}
	return len(matches), nil
}

// describe words the update: the job, how long it's been out, and why it
// matches.
func describe(match store.FreshMatch, now time.Time) store.NewUpdate {
	title := "Strong match: " + match.JobTitle
	if match.CompanyName != nil && *match.CompanyName != "" {
		title += " at " + *match.CompanyName
	}
	age := "Found " + formatAge(now.Sub(match.FirstSeenAt)) + "."
	if match.PublishedAt != nil {
		age = "Posted " + formatAge(now.Sub(*match.PublishedAt)) + "."
	}
	body := []string{age}
	if reason := strings.TrimSpace(match.Reason); reason != "" {
		body = append(body, reason)
	}
	jobID := match.JobID
	return store.NewUpdate{Kind: Kind, Title: title, Body: strings.Join(body, " "), JobID: &jobID, CompanyID: match.CompanyID}
}

// formatAge words how long ago something happened, in the largest whole
// unit.
func formatAge(age time.Duration) string {
	switch {
	case age < time.Hour:
		return "within the hour"
	case age < 24*time.Hour:
		return plural(int(age/time.Hour), "hour") + " ago"
	default:
		return plural(int(age/(24*time.Hour)), "day") + " ago"
	}
}

func plural(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", count, unit)
}
