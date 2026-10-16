package store

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WriteMark is every table's write counters at one moment, read from
// Postgres's own statistics, which count every writer's inserts, updates
// and deletes: the store's, the migrations' and anyone else's. A new
// version's server takes it after migrating and before it serves anything,
// so a rollback can count what that version wrote, which restoring the
// pre-migration dump loses.
type WriteMark struct {
	At time.Time `json:"at"`
	// DatabaseStarted is the Postgres server's start, for the record: the
	// server the hub owns restarts with every hub-server.
	DatabaseStarted time.Time `json:"database_started"`
	// StatsReset is the database's last statistics reset, by
	// pg_stat_reset(); nil when it was never reset.
	StatsReset *time.Time `json:"stats_reset,omitempty"`
	// SharedStatsReset is the cluster's, which Postgres also resets when it
	// recovers from a crash and drops every counter.
	SharedStatsReset *time.Time               `json:"shared_stats_reset,omitempty"`
	Tables           map[string]TableCounters `json:"tables"`
}

// TableCounters are a table's rows inserted, updated and deleted since the
// statistics were last reset.
type TableCounters struct {
	Inserted int64 `json:"inserted"`
	Updated  int64 `json:"updated"`
	Deleted  int64 `json:"deleted"`
}

// ReadWriteMark reads every table's counters. Only what other backends have
// flushed shows: Postgres flushes a backend's counts about once a second
// while it works, when it goes idle a while later, and when it exits.
func ReadWriteMark(ctx context.Context, pool *pgxpool.Pool) (WriteMark, error) {
	mark := WriteMark{Tables: map[string]TableCounters{}}
	err := pool.QueryRow(ctx, `
		SELECT now(), pg_postmaster_start_time(),
			(SELECT stats_reset FROM pg_stat_database WHERE datname = current_database()),
			(SELECT stats_reset FROM pg_stat_bgwriter)`,
	).Scan(&mark.At, &mark.DatabaseStarted, &mark.StatsReset, &mark.SharedStatsReset)
	if err != nil {
		return WriteMark{}, fmt.Errorf("read the database's statistics: %w", err)
	}
	rows, err := pool.Query(ctx, `
		SELECT relname, n_tup_ins, n_tup_upd, n_tup_del
		FROM pg_stat_user_tables
		WHERE schemaname = current_schema()`)
	if err != nil {
		return WriteMark{}, fmt.Errorf("read the tables' write counters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var counters TableCounters
		if err := rows.Scan(&name, &counters.Inserted, &counters.Updated, &counters.Deleted); err != nil {
			return WriteMark{}, err
		}
		mark.Tables[name] = counters
	}
	if err := rows.Err(); err != nil {
		return WriteMark{}, fmt.Errorf("read the tables' write counters: %w", err)
	}
	return mark, nil
}

// WriteCount is what was written between a mark and a later reading, table
// by table, or why it couldn't be counted.
type WriteCount struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	// Uncountable says why the counters can't be trusted: they were reset,
	// Postgres crashed, or one went down. Empty when they can.
	Uncountable string `json:"uncountable,omitempty"`
	// Tables are the tables written to, by name.
	Tables []TableWrites `json:"tables,omitempty"`
}

// TableWrites are the rows a table had written to it since the mark.
type TableWrites struct {
	Table string `json:"table"`
	TableCounters
}

// CountWrites reads the counters now, and subtracts mark from them.
func CountWrites(ctx context.Context, pool *pgxpool.Pool, mark WriteMark) (WriteCount, error) {
	now, err := ReadWriteMark(ctx, pool)
	if err != nil {
		return WriteCount{}, err
	}
	return CompareWriteMarks(mark, now), nil
}

// CompareWriteMarks counts the writes between two readings of the
// counters. It doesn't guess: when the statistics were reset or dropped in
// between, or any counter went down, the count says it couldn't be made.
func CompareWriteMarks(mark, now WriteMark) WriteCount {
	count := WriteCount{From: mark.At, To: now.At}
	switch {
	case !sameTime(mark.StatsReset, now.StatsReset):
		count.Uncountable = "the database's statistics were reset"
		return count
	case !sameTime(mark.SharedStatsReset, now.SharedStatsReset):
		count.Uncountable = "Postgres dropped its statistics, as it does after a crash"
		return count
	}
	for _, name := range slices.Sorted(maps.Keys(now.Tables)) {
		after := now.Tables[name]
		before := mark.Tables[name]
		writes := TableWrites{Table: name, TableCounters: TableCounters{
			Inserted: after.Inserted - before.Inserted,
			Updated:  after.Updated - before.Updated,
			Deleted:  after.Deleted - before.Deleted,
		}}
		if writes.Inserted < 0 || writes.Updated < 0 || writes.Deleted < 0 {
			return WriteCount{From: mark.At, To: now.At, Uncountable: "the write counters of " + name + " went down"}
		}
		if writes.Inserted > 0 || writes.Updated > 0 || writes.Deleted > 0 {
			count.Tables = append(count.Tables, writes)
		}
	}
	return count
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

// Lost is what the owner would lose with the writes counted, in their
// words, the most first: "3 jobs found", "1 card moved". Bookkeeping isn't
// in it, and a table without words counts as "other changes".
func (count WriteCount) Lost() []string {
	totals := map[Phrase]int64{}
	add := func(phrase Phrase, rows int64) {
		if rows > 0 && phrase.One != "" {
			totals[phrase] += rows
		}
	}
	for _, writes := range count.Tables {
		if BookkeepingTables[writes.Table] {
			continue
		}
		words, ok := TableWordsByName[writes.Table]
		if !ok {
			words = TableWords{Added: otherChanges, Changed: otherChanges}
		}
		add(words.Added, writes.Inserted)
		add(words.Changed, writes.Updated+writes.Deleted)
	}
	phrases := slices.SortedFunc(maps.Keys(totals), func(left, right Phrase) int {
		if order := cmp.Compare(totals[right], totals[left]); order != 0 {
			return order
		}
		return strings.Compare(left.Many, right.Many)
	})
	lost := make([]string, 0, len(phrases))
	for _, phrase := range phrases {
		lost = append(lost, phrase.Count(totals[phrase]))
	}
	return lost
}

// IsNothingLost reports whether the writes counted are only bookkeeping.
func (count WriteCount) IsNothingLost() bool {
	return count.Uncountable == "" && len(count.Lost()) == 0
}

// Describe says what the writes cost, in a sentence: "Nothing was lost.",
// "Lost: 3 jobs found and 1 card moved.", or, when they couldn't be
// counted, "Anything written between 03:31 and 03:33 couldn't be counted,
// and was lost." Times are in location.
func (count WriteCount) Describe(location *time.Location) string {
	if count.Uncountable != "" {
		return fmt.Sprintf("Anything written between %s and %s couldn't be counted, and was lost.",
			count.From.In(location).Format("15:04"), count.To.In(location).Format("15:04"))
	}
	lost := count.Lost()
	switch len(lost) {
	case 0:
		return "Nothing was lost."
	case 1:
		return "Lost: " + lost[0] + "."
	default:
		return "Lost: " + strings.Join(lost[:len(lost)-1], ", ") + " and " + lost[len(lost)-1] + "."
	}
}

// Phrase names rows the way the owner says them, for one and for many.
type Phrase struct {
	One, Many string
}

// Count puts the number before the phrase: "1 card moved", "3 cards moved".
func (phrase Phrase) Count(rows int64) string {
	if rows == 1 {
		return "1 " + phrase.One
	}
	return fmt.Sprintf("%d %s", rows, phrase.Many)
}

// TableWords name a table's writes: its rows added, and its rows changed
// or deleted. A table whose changes are bookkeeping, such as a phone's
// last_seen_at, has no Changed words.
type TableWords struct {
	Added   Phrase
	Changed Phrase
}

var otherChanges = Phrase{One: "other change", Many: "other changes"}

func words(addedOne, addedMany, changedOne, changedMany string) TableWords {
	return TableWords{Added: Phrase{addedOne, addedMany}, Changed: Phrase{changedOne, changedMany}}
}

// TableWordsByName are the words for each table whose writes the owner
// would miss. Every table is either here or in BookkeepingTables, and a
// store test fails on a new table that is in neither.
var TableWordsByName = map[string]TableWords{
	"agent_prompts":             words("prompt version saved", "prompt versions saved", "prompt changed", "prompts changed"),
	"agent_runs":                words("agent run", "agent runs", "agent run finished", "agent runs finished"),
	"application_answers":       words("application answer saved", "application answers saved", "application answer changed", "application answers changed"),
	"applications":              words("card added", "cards added", "card moved", "cards moved"),
	"artifacts":                 words("file added", "files added", "file changed", "files changed"),
	"claude_sessions":           words("Claude session", "Claude sessions", "", ""),
	"companies":                 words("company added", "companies added", "company changed", "companies changed"),
	"comparison_answers":        words("comparison answer", "comparison answers", "comparison answer", "comparison answers"),
	"comparison_verdicts":       words("comparison verdict", "comparison verdicts", "comparison verdict", "comparison verdicts"),
	"comparisons":               words("comparison started", "comparisons started", "comparison changed", "comparisons changed"),
	"connections":               words("LinkedIn connection imported", "LinkedIn connections imported", "LinkedIn connection changed", "LinkedIn connections changed"),
	"cv_screens":                words("CV screen", "CV screens", "CV screen", "CV screens"),
	"cvs":                       words("CV drafted", "CVs drafted", "CV edited", "CVs edited"),
	"devices":                   words("phone paired", "phones paired", "", ""),
	"google_connection":         words("Google sign-in", "Google sign-ins", "Google sign-in change", "Google sign-in changes"),
	"interview_packs":           words("interview pack", "interview packs", "interview pack", "interview packs"),
	"job_boards":                words("job board found", "job boards found", "", ""),
	"job_briefs":                words("job brief", "job briefs", "job brief rewritten", "job briefs rewritten"),
	"job_criteria":              words("job criteria change", "job criteria changes", "job criteria change", "job criteria changes"),
	"job_decisions":             words("job decision", "job decisions", "job decision", "job decisions"),
	"job_facts":                 words("job's facts read", "jobs' facts read", "job's facts read", "jobs' facts read"),
	"jobs":                      words("job found", "jobs found", "job changed", "jobs changed"),
	"linkedin_company_follows":  words("LinkedIn export item", "LinkedIn export items", "LinkedIn export item", "LinkedIn export items"),
	"linkedin_conversations":    words("LinkedIn conversation", "LinkedIn conversations", "LinkedIn conversation", "LinkedIn conversations"),
	"linkedin_endorsements":     words("LinkedIn export item", "LinkedIn export items", "LinkedIn export item", "LinkedIn export items"),
	"linkedin_invitations":      words("LinkedIn export item", "LinkedIn export items", "LinkedIn export item", "LinkedIn export items"),
	"linkedin_messages":         words("LinkedIn message", "LinkedIn messages", "LinkedIn message", "LinkedIn messages"),
	"linkedin_profile":          words("LinkedIn export item", "LinkedIn export items", "LinkedIn export item", "LinkedIn export items"),
	"linkedin_recommendations":  words("LinkedIn export item", "LinkedIn export items", "LinkedIn export item", "LinkedIn export items"),
	"mail_messages":             words("email", "emails", "email sorted", "emails sorted"),
	"model_providers":           words("model provider", "model providers", "model provider", "model providers"),
	"model_work_settings":       words("model setting change", "model setting changes", "model setting change", "model setting changes"),
	"network_contact_companies": words("contact note", "contact notes", "contact note", "contact notes"),
	"network_contacts":          words("contact added", "contacts added", "contact changed", "contacts changed"),
	"owner_profile":             words("profile edit", "profile edits", "profile edit", "profile edits"),
	"people":                    words("person added", "people added", "person changed", "people changed"),
	"pipeline_phases":           words("pipeline phase change", "pipeline phase changes", "pipeline phase change", "pipeline phase changes"),
	"profile_entries":           words("profile entry added", "profile entries added", "profile entry changed", "profile entries changed"),
	"task_requests":             words("task asked of the Mac", "tasks asked of the Mac", "", ""),
	"task_routes":               words("model route change", "model route changes", "model route change", "model route changes"),
	"updates":                   words("update", "updates", "", ""),
	"watch_list_entries":        words("company watched", "companies watched", "watch list change", "watch list changes"),
}

// BookkeepingTables are the tables whose writes the owner loses nothing
// by: progress the next pass redoes, logs, caches replaced whole, and the
// rows that only go with another table's.
var BookkeepingTables = map[string]bool{
	// The change log is written beside the writes it names, which count
	// under their own tables.
	"changes": true,
	// The migrations' own records, which the restored dump brings back.
	"goose_db_version":   true,
	"migration_releases": true,
	// Where a pass got to; the next pass picks it up again.
	"board_index_pages":       true,
	"board_searches":          true,
	"discovered_board_tokens": true,
	"gallery_companies":       true,
	"gallery_list_reads":      true,
	"hiring_thread_comments":  true,
	"posting_text_searches":   true,
	// Replaced whole by each refresh.
	"market_gaps": true,
	// The log of model calls; what they answered is kept where it's used.
	"task_runs": true,
	// A comparison's jobs and stacks are written with it, which counts
	// under comparisons.
	"comparison_jobs":   true,
	"comparison_stacks": true,
}
