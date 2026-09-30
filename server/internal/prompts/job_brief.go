package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// maximumRoleBodyLength and maximumEntryBodyLength keep the knowledge base
	// within a local model's context, beside the posting.
	maximumRoleBodyLength  = 500
	maximumEntryBodyLength = 300
)

// JobBriefPrompt is the job_brief prompt with the knowledge base and criteria
// filled in, and the references its entries carry: a brief cites "E12", and
// EntryRefs turns it back into the entry.
type JobBriefPrompt struct {
	Rendered
	EntryRefs map[string]uuid.UUID
}

// RenderJobBriefPrompt fills the prompt's {{knowledge_base}} with every
// knowledge-base entry, each marked with a reference, and {{job_criteria}}
// with the roles, levels and stack the owner is after.
func RenderJobBriefPrompt(ctx context.Context, hub *store.Store, prompt store.AgentPrompt) (JobBriefPrompt, error) {
	entries, err := hub.ListProfileEntries(ctx, store.ProfileEntryFilter{})
	if err != nil {
		return JobBriefPrompt{}, err
	}
	saved, err := hub.GetJobCriteria(ctx)
	if err != nil {
		return JobBriefPrompt{}, err
	}
	knowledgeBase, refs := formatKnowledgeBase(entries)
	filled := strings.NewReplacer("{{knowledge_base}}", knowledgeBase, "{{job_criteria}}", formatBriefCriteria(saved.Criteria)).Replace(prompt.Body)
	return JobBriefPrompt{Rendered: Rendered{Body: filled, Version: prompt.Version}, EntryRefs: refs}, nil
}

// formatKnowledgeBase lists the entries compactly, each after its reference:
// roles newest first with their cases, skills and projects beneath them,
// then the entries tied to no role, by kind.
func formatKnowledgeBase(entries []store.ProfileEntry) (string, map[string]uuid.UUID) {
	refs := map[string]uuid.UUID{}
	var text strings.Builder
	write := func(entry store.ProfileEntry, indent string) {
		ref := fmt.Sprintf("E%d", len(refs)+1)
		refs[ref] = entry.ID
		fmt.Fprintf(&text, "%s[%s] %s\n", indent, ref, describeEntry(entry))
	}
	childrenByRole := map[uuid.UUID][]store.ProfileEntry{}
	var unattached []store.ProfileEntry
	for _, entry := range entries {
		switch {
		case entry.Kind == "role":
		case entry.RoleID != nil:
			childrenByRole[*entry.RoleID] = append(childrenByRole[*entry.RoleID], entry)
		default:
			unattached = append(unattached, entry)
		}
	}
	for _, entry := range entries {
		if entry.Kind != "role" {
			continue
		}
		write(entry, "")
		for _, child := range childrenByRole[entry.ID] {
			write(child, "  ")
		}
	}
	for _, kind := range []string{"case", "project", "skill", "education", "fact", "preference"} {
		for _, entry := range unattached {
			if entry.Kind == kind {
				write(entry, "")
			}
		}
	}
	if text.Len() == 0 {
		return "The knowledge base is empty.", refs
	}
	return dataPreamble + "\n\n" + text.String(), refs
}

// describeEntry is one entry on one line: its kind and title, where and
// when, a trimmed body, its outcome and skills.
func describeEntry(entry store.ProfileEntry) string {
	parts := []string{strings.ToUpper(entry.Kind[:1]) + entry.Kind[1:] + ": " + entry.Title}
	if entry.Organization != "" {
		parts[0] += " at " + entry.Organization
	}
	if entry.StartMonth != "" || entry.EndMonth != "" {
		end := entry.EndMonth
		if end == "" && entry.Kind == "role" {
			end = "present"
		}
		parts[0] += fmt.Sprintf(" (%s – %s)", entry.StartMonth, end)
	}
	limit := maximumEntryBodyLength
	if entry.Kind == "role" {
		limit = maximumRoleBodyLength
	}
	if body := shortenText(strings.Join(strings.Fields(entry.Body), " "), limit); body != "" {
		parts = append(parts, body)
	}
	if entry.Outcome != "" {
		parts = append(parts, "Outcome: "+shortenText(entry.Outcome, maximumEntryBodyLength))
	}
	if len(entry.Skills) > 0 {
		parts = append(parts, "Skills: "+strings.Join(entry.Skills, ", "))
	}
	return strings.Join(parts, ". ")
}

func shortenText(text string, length int) string {
	if len(text) <= length {
		return text
	}
	return strings.ToValidUTF8(text[:length], "") + "…"
}

// formatBriefCriteria is what the owner is after, as the brief weighs it.
func formatBriefCriteria(criteria store.JobCriteria) string {
	return fmt.Sprintf("Roles: %s\nLevels: %s\nStack: %s",
		strings.Join(criteria.Roles, "; "), strings.Join(criteria.SeniorityLevels, ", "), strings.Join(criteria.Technologies, ", "))
}
