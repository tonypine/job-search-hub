package store

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// How a person can get the owner in.
const (
	// RelationContact is someone an agent found at a company and stored in
	// its dossier.
	RelationContact = "contact"
	// RelationConnection is a LinkedIn connection who works at a hub
	// company.
	RelationConnection = "connection"
	// RelationIntroducer is someone the owner knows who doesn't work at a
	// company but can open doors there: a warm path.
	RelationIntroducer = "introducer"
	// RelationRecruiter is someone who started a LinkedIn conversation to
	// recruit the owner.
	RelationRecruiter = "recruiter"
)

// RelatedPerson is one person who can get the owner in, whatever the hub
// knows them from, with their relation. An introducer linked to two
// companies is two people, one at each.
type RelatedPerson struct {
	// Key tells people apart across relations, e.g. "recruiter:<id>".
	Key      string `json:"key"`
	Relation string `json:"relation"`
	// ID is the person's id where the relation keeps them: a dossier
	// person, a connection, a network contact, or the conversation a
	// recruiter started.
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Role        string     `json:"role,omitempty"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	CompanyName string     `json:"company_name,omitempty"`
	ProfileURL  string     `json:"profile_url,omitempty"`
	Email       string     `json:"email,omitempty"`
	// SourceURL is the page that named a contact.
	SourceURL string `json:"source_url,omitempty"`
	// Note says how they can help: a contact's notes, how an introducer can
	// help at the company, or why a conversation was read as a recruiter's.
	Note string `json:"note,omitempty"`
	// Relevance is why a contact is worth contacting, one of
	// PersonRelevances.
	Relevance string `json:"relevance,omitempty"`
	// Closeness is how close the owner is to a connection.
	Closeness string `json:"closeness,omitempty"`
	// HowKnown and PreferredChannel are an introducer's.
	HowKnown         string `json:"how_known,omitempty"`
	PreferredChannel string `json:"preferred_channel,omitempty"`
	// HiringRole and IsAgency are what a recruiter hires for, and whether
	// they work at an agency.
	HiringRole string `json:"hiring_role,omitempty"`
	IsAgency   bool   `json:"is_agency"`
	// LastContactAt is the latest message between them and the owner.
	LastContactAt *time.Time `json:"last_contact_at,omitempty"`
	// Answered says whether the owner wrote back, for a conversation the
	// other person started; absent for anyone else.
	Answered *bool `json:"answered,omitempty"`
}

var relationOrder = []string{RelationRecruiter, RelationIntroducer, RelationConnection, RelationContact}

// ListRelatedPeople returns everyone who can get the owner in, at one
// company or at all of them when companyID is nil: the dossiers' contacts,
// the connections who work at a hub company, the introducers, and the
// recruiters, at their company when the hub holds it. The latest contacted
// come first, then the rest by relation and name.
func (s *Store) ListRelatedPeople(ctx context.Context, companyID *uuid.UUID) ([]RelatedPerson, error) {
	people := []RelatedPerson{}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		for _, list := range []func(context.Context, pgx.Tx, *uuid.UUID) ([]RelatedPerson, error){
			listContacts, listConnectionsAtCompanies, listIntroducers, listRecruiters,
		} {
			listed, err := list(ctx, tx, companyID)
			if err != nil {
				return err
			}
			people = append(people, listed...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(people, compareRelatedPeople)
	return people, nil
}

func compareRelatedPeople(a, b RelatedPerson) int {
	switch {
	case a.LastContactAt != nil && b.LastContactAt != nil:
		if order := b.LastContactAt.Compare(*a.LastContactAt); order != 0 {
			return order
		}
	case a.LastContactAt != nil:
		return -1
	case b.LastContactAt != nil:
		return 1
	}
	return cmp.Or(
		cmp.Compare(slices.Index(relationOrder, a.Relation), slices.Index(relationOrder, b.Relation)),
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(strings.ToLower(a.CompanyName), strings.ToLower(b.CompanyName)),
		cmp.Compare(a.Key, b.Key),
	)
}

func listContacts(ctx context.Context, tx pgx.Tx, companyID *uuid.UUID) ([]RelatedPerson, error) {
	rows, err := tx.Query(ctx, `
		SELECT people.id, people.name, people.role_title, people.relevance, people.profile_url, people.source_url, people.notes,
			people.email, companies.id, companies.name
		FROM people JOIN companies ON companies.id = people.company_id
		WHERE $1::uuid IS NULL OR people.company_id = $1`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (RelatedPerson, error) {
		person := RelatedPerson{Relation: RelationContact, CompanyID: new(uuid.UUID)}
		err := row.Scan(&person.ID, &person.Name, &person.Role, &person.Relevance, &person.ProfileURL, &person.SourceURL, &person.Note,
			&person.Email, person.CompanyID, &person.CompanyName)
		person.Key = RelationContact + ":" + person.ID.String()
		return person, err
	})
}

func listConnectionsAtCompanies(ctx context.Context, tx pgx.Tx, companyID *uuid.UUID) ([]RelatedPerson, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+connectionColumns+` FROM connections
		WHERE company_id IS NOT NULL AND ($1::uuid IS NULL OR company_id = $1)`, companyID)
	if err != nil {
		return nil, err
	}
	connections, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Connection, error) { return scanConnection(row) })
	if err != nil {
		return nil, err
	}
	companyNames, err := getCompanyNamesByID(ctx, tx)
	if err != nil {
		return nil, err
	}
	people := make([]RelatedPerson, 0, len(connections))
	for _, connection := range rankByCloseness(connections, time.Now()) {
		people = append(people, RelatedPerson{
			Key: RelationConnection + ":" + connection.ID.String(), Relation: RelationConnection, ID: connection.ID,
			Name: strings.TrimSpace(connection.FirstName + " " + connection.LastName), Role: connection.Position,
			CompanyID: connection.CompanyID, CompanyName: companyNames[*connection.CompanyID], ProfileURL: connection.ProfileURL,
			Email: connection.Email, Closeness: connection.Closeness, LastContactAt: connection.LastMessageAt,
		})
	}
	return people, nil
}

func listIntroducers(ctx context.Context, tx pgx.Tx, companyID *uuid.UUID) ([]RelatedPerson, error) {
	rows, err := tx.Query(ctx, `
		SELECT contacts.id, contacts.name, contacts.how_known, contacts.preferred_channel, links.note, companies.id, companies.name
		FROM network_contact_companies AS links
		JOIN network_contacts AS contacts ON contacts.id = links.contact_id
		JOIN companies ON companies.id = links.company_id
		WHERE $1::uuid IS NULL OR links.company_id = $1`, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (RelatedPerson, error) {
		person := RelatedPerson{Relation: RelationIntroducer, CompanyID: new(uuid.UUID)}
		err := row.Scan(&person.ID, &person.Name, &person.HowKnown, &person.PreferredChannel, &person.Note, person.CompanyID, &person.CompanyName)
		person.Role = person.HowKnown
		person.Key = RelationIntroducer + ":" + person.ID.String() + ":" + person.CompanyID.String()
		return person, err
	})
}

// listRecruiters returns the recruiters' conversations as people, at the
// company they hire for, or else their own employer, under the hub's name
// for it when the hub holds it.
func listRecruiters(ctx context.Context, tx pgx.Tx, companyID *uuid.UUID) ([]RelatedPerson, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+linkedInConversationColumns+` FROM `+linkedInConversationJoin+`
		WHERE conversations.classification = 'recruiter_outreach'`)
	if err != nil {
		return nil, err
	}
	conversations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LinkedInConversation, error) { return scanLinkedInConversation(row) })
	if err != nil {
		return nil, err
	}
	companyIDs, err := getCompanyIDsByName(ctx, tx)
	if err != nil {
		return nil, err
	}
	companyNames, err := getCompanyNamesByID(ctx, tx)
	if err != nil {
		return nil, err
	}
	people := []RelatedPerson{}
	for _, conversation := range conversations {
		person := RelatedPerson{
			Key: RelationRecruiter + ":" + conversation.ID.String(), Relation: RelationRecruiter, ID: conversation.ID,
			Name: conversation.StartedByName, Role: derefOrEmpty(conversation.StarterPosition), CompanyName: conversation.HiringCompany,
			ProfileURL: conversation.StartedByURL, Note: conversation.ClassificationReason, HiringRole: conversation.Role,
			IsAgency: conversation.IsAgency, LastContactAt: conversation.LastMessageAt, Answered: &conversation.OwnerWrote,
		}
		if person.CompanyName == "" {
			person.CompanyName = derefOrEmpty(conversation.StarterCompany)
		}
		if found, ok := companyIDs[NormalizeCompanyName(person.CompanyName)]; ok && person.CompanyName != "" {
			person.CompanyID, person.CompanyName = &found, companyNames[found]
		}
		if companyID == nil || (person.CompanyID != nil && *person.CompanyID == *companyID) {
			people = append(people, person)
		}
	}
	return people, nil
}

func getCompanyNamesByID(ctx context.Context, tx pgx.Tx) (map[uuid.UUID]string, error) {
	rows, err := tx.Query(ctx, `SELECT id, name FROM companies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	return names, rows.Err()
}

func derefOrEmpty(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}
