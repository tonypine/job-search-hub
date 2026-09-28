package mcptools

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type createCompanyInput struct {
	Name       string `json:"name" jsonschema:"the company's name as it presents itself"`
	Domain     string `json:"domain" jsonschema:"the company's own domain, or any URL on it; stored normalized, e.g. stripe.com"`
	WebsiteURL string `json:"website_url,omitempty" jsonschema:"the company's homepage"`
	SourceURL  string `json:"source_url,omitempty" jsonschema:"the page these facts came from"`
}

type createCompanyOutput struct {
	Company store.Company `json:"company"`
	Created bool          `json:"created" jsonschema:"false when a company with this domain was already stored; it is returned unchanged"`
}

type getCompanyInput struct {
	CompanyID *uuid.UUID `json:"company_id,omitempty" jsonschema:"the company's id; give this or domain"`
	Domain    string     `json:"domain,omitempty" jsonschema:"the company's domain or any URL on it; give this or company_id"`
}

// companyDossier is everything the hub knows about one company.
type companyDossier struct {
	Company      store.Company `json:"company"`
	WatchedSince *time.Time    `json:"watched_since,omitempty" jsonschema:"when the company was put on the watch list; absent when it is not on it"`
}

type findCompaniesInput struct {
	Query string `json:"query" jsonschema:"part of a company's name, or its domain or a URL on it"`
}

type findCompaniesOutput struct {
	Companies []store.Company `json:"companies"`
}

type updateCompanyInput struct {
	CompanyID           uuid.UUID `json:"company_id" jsonschema:"the company to update"`
	Name                *string   `json:"name,omitempty"`
	WebsiteURL          *string   `json:"website_url,omitempty"`
	CareersURL          *string   `json:"careers_url,omitempty" jsonschema:"the page listing open roles"`
	HeadquartersCountry *string   `json:"headquarters_country,omitempty"`
	EmployeeCountRange  *string   `json:"employee_count_range,omitempty" jsonschema:"e.g. 51-200"`
	Summary             *string   `json:"summary,omitempty" jsonschema:"two or three sentences on what the company does"`
	SourceURL           string    `json:"source_url,omitempty" jsonschema:"the page these facts came from"`
}

func addCompanyTools(server *mcp.Server, hub *store.Store) {
	addTool(server, &mcp.Tool{
		Name:        "create_company",
		Description: "Store a company. When a company with the same domain is already stored, it is returned unchanged with created=false, so call find_companies first when unsure.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input createCompanyInput) (*mcp.CallToolResult, createCompanyOutput, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, createCompanyOutput{}, err
		}
		company, created, err := hub.CreateCompany(ctx, actor, store.NewCompany{
			Name: input.Name, Domain: input.Domain, WebsiteURL: input.WebsiteURL, SourceURL: input.SourceURL,
		})
		return nil, createCompanyOutput{Company: company, Created: created}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "get_company",
		Description: "Get a stored company's dossier by id or domain.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getCompanyInput) (*mcp.CallToolResult, companyDossier, error) {
		var company store.Company
		var err error
		switch {
		case input.CompanyID != nil:
			company, err = hub.GetCompany(ctx, *input.CompanyID)
		case input.Domain != "":
			company, err = hub.GetCompanyByDomain(ctx, input.Domain)
		default:
			err = errors.New("give company_id or domain")
		}
		if err != nil {
			return nil, companyDossier{}, err
		}
		watchedSince, err := hub.GetWatchedSince(ctx, company.ID)
		return nil, companyDossier{Company: company, WatchedSince: watchedSince}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "find_companies",
		Description: "Search stored companies by part of the name, or by domain or a URL on it.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input findCompaniesInput) (*mcp.CallToolResult, findCompaniesOutput, error) {
		companies, err := hub.FindCompanies(ctx, input.Query)
		return nil, findCompaniesOutput{Companies: companies}, err
	})

	addTool(server, &mcp.Tool{
		Name:        "update_company",
		Description: "Set any of a company's descriptive fields. Only the fields given are changed; the domain cannot be changed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateCompanyInput) (*mcp.CallToolResult, store.Company, error) {
		actor, err := tokens.GetActor(ctx)
		if err != nil {
			return nil, store.Company{}, err
		}
		company, err := hub.UpdateCompany(ctx, actor, input.CompanyID, store.CompanyUpdate{
			Name:                input.Name,
			WebsiteURL:          input.WebsiteURL,
			CareersURL:          input.CareersURL,
			HeadquartersCountry: input.HeadquartersCountry,
			EmployeeCountRange:  input.EmployeeCountRange,
			Summary:             input.Summary,
			SourceURL:           input.SourceURL,
		})
		return nil, company, err
	})
}
