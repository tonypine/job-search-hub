package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

// Connection is someone in the owner's LinkedIn network, and the hub company
// they work at when its name matches.
type Connection struct {
	ID          uuid.UUID  `json:"id"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	ProfileURL  string     `json:"profile_url"`
	Email       string     `json:"email,omitempty"`
	CompanyName string     `json:"company_name,omitempty"`
	Position    string     `json:"position,omitempty"`
	ConnectedOn *time.Time `json:"connected_on,omitempty"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	ImportedAt  time.Time  `json:"imported_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// The owner's LinkedIn conversations with them: how many, how many
	// messages, when first and last, and whether they ever wrote first.
	ConversationCount int        `json:"conversation_count"`
	MessageCount      int        `json:"message_count"`
	FirstMessageAt    *time.Time `json:"first_message_at,omitempty"`
	LastMessageAt     *time.Time `json:"last_message_at,omitempty"`
	TheyWroteFirst    bool       `json:"they_wrote_first"`
	// Vouches between them and the owner: the skills they endorsed the owner
	// for, and recommendations either way.
	EndorsedOwnerFor []string `json:"endorsed_owner_for,omitempty"`
	OwnerEndorsed    bool     `json:"owner_endorsed"`
	RecommendedOwner bool     `json:"recommended_owner"`
	OwnerRecommended bool     `json:"owner_recommended"`
	// Closeness says how close the owner is to them, e.g. "12 messages, last
	// in Mar 2025" or "connected in 2013, never talked"; connections are
	// listed closest first.
	Closeness string `json:"closeness"`
}

// rankByCloseness orders connections closest first: the ones the owner has
// talked with, the more and the more recently the closer, then the ones
// never talked with, the longest connected first. It fills each Closeness.
func rankByCloseness(connections []Connection, now time.Time) []Connection {
	for index := range connections {
		connections[index].Closeness = describeCloseness(connections[index])
	}
	sort.SliceStable(connections, func(a, b int) bool {
		return getClosenessScore(connections[a], now) > getClosenessScore(connections[b], now)
	})
	return connections
}

func getClosenessScore(connection Connection, now time.Time) float64 {
	return getConversationScore(connection, now) + getVouchScore(connection)
}

// getVouchScore weighs vouching: a recommendation either way says they know
// the owner's work well, an endorsement of the owner says they vouch for a
// skill, and the owner's endorsement of them says the owner knows theirs.
func getVouchScore(connection Connection) float64 {
	score := 0.0
	if connection.RecommendedOwner || connection.OwnerRecommended {
		score += 25
	}
	if len(connection.EndorsedOwnerFor) > 0 {
		score += 12
	}
	if connection.OwnerEndorsed {
		score += 4
	}
	return score
}

func getConversationScore(connection Connection, now time.Time) float64 {
	if connection.MessageCount == 0 || connection.LastMessageAt == nil {
		if connection.ConnectedOn == nil {
			return 0
		}
		yearsConnected := now.Sub(*connection.ConnectedOn).Hours() / hoursPerYear
		return min(yearsConnected, 10) / 10
	}
	yearsSinceTalking := now.Sub(*connection.LastMessageAt).Hours() / hoursPerYear
	recency := max(0, 5-yearsSinceTalking) * 10
	return 10 + recency + float64(min(connection.MessageCount, 30))
}

const hoursPerYear = 24 * 365

func describeCloseness(connection Connection) string {
	parts := []string{describeConversations(connection)}
	switch {
	case connection.RecommendedOwner:
		parts = append(parts, "recommended you")
	case connection.OwnerRecommended:
		parts = append(parts, "you recommended them")
	}
	if len(connection.EndorsedOwnerFor) > 0 {
		skills := connection.EndorsedOwnerFor
		if len(skills) > 3 {
			skills = append(skills[:3:3], "more")
		}
		parts = append(parts, "endorsed you for "+strings.Join(skills, ", "))
	}
	return strings.Join(parts, "; ")
}

func describeConversations(connection Connection) string {
	if connection.MessageCount == 0 || connection.LastMessageAt == nil {
		if connection.ConnectedOn == nil {
			return "never talked"
		}
		return fmt.Sprintf("connected in %d, never talked", connection.ConnectedOn.Year())
	}
	messages := "1 message"
	if connection.MessageCount != 1 {
		messages = fmt.Sprintf("%d messages", connection.MessageCount)
	}
	return fmt.Sprintf("%s, last in %s", messages, connection.LastMessageAt.Format("Jan 2006"))
}

const connectionColumns = `id, first_name, last_name, profile_url, email, company_name, position, connected_on, company_id, imported_at, updated_at,
	conversation_count, message_count, first_message_at, last_message_at, they_wrote_first,
	endorsed_owner_for, owner_endorsed, recommended_owner, owner_recommended`

func scanConnection(row pgx.Row) (Connection, error) {
	var connection Connection
	err := row.Scan(&connection.ID, &connection.FirstName, &connection.LastName, &connection.ProfileURL, &connection.Email,
		&connection.CompanyName, &connection.Position, &connection.ConnectedOn, &connection.CompanyID, &connection.ImportedAt, &connection.UpdatedAt,
		&connection.ConversationCount, &connection.MessageCount, &connection.FirstMessageAt, &connection.LastMessageAt, &connection.TheyWroteFirst,
		&connection.EndorsedOwnerFor, &connection.OwnerEndorsed, &connection.RecommendedOwner, &connection.OwnerRecommended)
	return connection, err
}

type NewConnection struct {
	FirstName   string
	LastName    string
	ProfileURL  string
	Email       string
	CompanyName string
	Position    string
	ConnectedOn *time.Time
}

// ConnectionImport counts what an import did: connections added, known ones
// updated, and how many of the stored connections work at a hub company.
type ConnectionImport struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Matched int `json:"matched"`
}

// ImportConnections adds each connection, or updates the one stored under
// the same profile URL, then ties every connection to the hub company its
// company name matches. The change log records the import, not each person.
func (s *Store) ImportConnections(ctx context.Context, actor Actor, connections []NewConnection) (ConnectionImport, error) {
	var result ConnectionImport
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, connection := range connections {
			var inserted bool
			err := tx.QueryRow(ctx, `
				INSERT INTO connections (first_name, last_name, profile_url, email, company_name, position, connected_on)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (lower(profile_url)) DO UPDATE SET
					first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
					email = CASE WHEN EXCLUDED.email = '' THEN connections.email ELSE EXCLUDED.email END,
					company_name = EXCLUDED.company_name, position = EXCLUDED.position, connected_on = EXCLUDED.connected_on,
					updated_at = now()
				RETURNING xmax = 0`,
				connection.FirstName, connection.LastName, connection.ProfileURL, connection.Email, connection.CompanyName,
				connection.Position, connection.ConnectedOn).Scan(&inserted)
			if err != nil {
				return err
			}
			if inserted {
				result.Added++
			} else {
				result.Updated++
			}
		}
		matched, err := matchConnectionsToCompanies(ctx, tx)
		if err != nil {
			return err
		}
		result.Matched = matched
		if _, err := refreshConnectionHistory(ctx, tx); err != nil {
			return err
		}
		if _, err := refreshConnectionVouches(ctx, tx); err != nil {
			return err
		}
		return insertChange(ctx, tx, actor, change{entityType: "connections", entityID: uuid.New(), operation: "import", after: result})
	})
	return result, err
}

// getCompanyIDsByName maps each company's name, as NormalizeCompanyName
// writes it, to the company.
func getCompanyIDsByName(ctx context.Context, tx pgx.Tx) (map[string]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id, name FROM companies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	companyIDs := map[string]uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		companyIDs[NormalizeCompanyName(name)] = id
	}
	return companyIDs, rows.Err()
}

// matchConnectionsToCompanies ties each connection to the company whose
// name, without case, accents or a legal suffix, equals theirs, and returns
// how many connections are tied to one.
func matchConnectionsToCompanies(ctx context.Context, tx pgx.Tx) (int, error) {
	companyIDs, err := getCompanyIDsByName(ctx, tx)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, company_name FROM connections`)
	if err != nil {
		return 0, err
	}
	matches := map[uuid.UUID]*uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		var companyName string
		if err := rows.Scan(&id, &companyName); err != nil {
			return 0, err
		}
		if companyID, found := companyIDs[NormalizeCompanyName(companyName)]; found && companyName != "" {
			matches[id] = &companyID
		} else {
			matches[id] = nil
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	matched := 0
	for id, companyID := range matches {
		if _, err := tx.Exec(ctx, `UPDATE connections SET company_id = $2 WHERE id = $1 AND company_id IS DISTINCT FROM $2`, id, companyID); err != nil {
			return 0, err
		}
		if companyID != nil {
			matched++
		}
	}
	return matched, nil
}

var legalSuffixes = []string{" inc", " inc.", " llc", " ltd", " ltd.", " ltda", " ltda.", " gmbh", " s.a.", " sa", " corp", " corp.", " co."}

// NormalizeCompanyName compares company names as people write them:
// "Acme, Inc." and "acme" are the same company.
func NormalizeCompanyName(name string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(wordmatch.Normalize(name), ",", ""))
	for _, suffix := range legalSuffixes {
		normalized = strings.TrimSuffix(normalized, suffix)
	}
	return strings.TrimSpace(normalized)
}

// ConnectionsSummary is how many connections the hub keeps and when they
// were last imported.
type ConnectionsSummary struct {
	Count          int        `json:"count"`
	Matched        int        `json:"matched"`
	LastImportedAt *time.Time `json:"last_imported_at,omitempty"`
	// Conversations and Invitations count what the archive import stored.
	Conversations int `json:"conversations"`
	Invitations   int `json:"invitations"`
}

func (s *Store) GetConnectionsSummary(ctx context.Context) (ConnectionsSummary, error) {
	var summary ConnectionsSummary
	err := s.pool.QueryRow(ctx, `
		SELECT count(*), count(company_id), max(updated_at),
			(SELECT count(*) FROM linkedin_conversations), (SELECT count(*) FROM linkedin_invitations)
		FROM connections`).
		Scan(&summary.Count, &summary.Matched, &summary.LastImportedAt, &summary.Conversations, &summary.Invitations)
	return summary, err
}

// ListCompanyConnections returns the connections who work at a company,
// the longest-standing first.
func (s *Store) ListCompanyConnections(ctx context.Context, companyID uuid.UUID) ([]Connection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+connectionColumns+` FROM connections WHERE company_id = $1 ORDER BY connected_on NULLS LAST, last_name`, companyID)
	if err != nil {
		return nil, err
	}
	connections, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Connection, error) { return scanConnection(row) })
	if connections == nil {
		connections = []Connection{}
	}
	return rankByCloseness(connections, time.Now()), err
}

// ListConnectionsAtCompanyName returns the connections who work at a company
// the hub doesn't hold, such as one a feed posting names, compared as
// ListCompanyConnections' matching compares names.
func (s *Store) ListConnectionsAtCompanyName(ctx context.Context, companyName string) ([]Connection, error) {
	wanted := NormalizeCompanyName(companyName)
	connections := []Connection{}
	if wanted == "" {
		return connections, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+connectionColumns+` FROM connections WHERE company_name <> '' ORDER BY connected_on NULLS LAST, last_name`)
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Connection, error) { return scanConnection(row) })
	if err != nil {
		return nil, err
	}
	for _, connection := range all {
		if NormalizeCompanyName(connection.CompanyName) == wanted {
			connections = append(connections, connection)
		}
	}
	return rankByCloseness(connections, time.Now()), nil
}

// tieJobsToCompanies gives each job that names its company without being
// tied to one, as a feed posting does, the hub company of that name, and
// moves the job's card with it. It returns how many jobs it tied.
func tieJobsToCompanies(ctx context.Context, tx pgx.Tx) (int, error) {
	companyIDs, err := getCompanyIDsByName(ctx, tx)
	if err != nil || len(companyIDs) == 0 {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, company_name FROM jobs WHERE company_id IS NULL AND company_name <> ''`)
	if err != nil {
		return 0, err
	}
	matches := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var jobID uuid.UUID
		var companyName string
		if err := rows.Scan(&jobID, &companyName); err != nil {
			return 0, err
		}
		if companyID, found := companyIDs[NormalizeCompanyName(companyName)]; found {
			matches[jobID] = companyID
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for jobID, companyID := range matches {
		if _, err := tx.Exec(ctx, `UPDATE jobs SET company_id = $2 WHERE id = $1`, jobID, companyID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET company_id = $2, updated_at = now() WHERE job_id = $1 AND company_id IS NULL`, jobID, companyID); err != nil {
			return 0, err
		}
	}
	return len(matches), nil
}
