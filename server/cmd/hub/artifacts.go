package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// attachFile uploads a file as context for the agents: about the owner, or
// about the company with companyDomain.
func attachFile(ctx context.Context, config cliConfig, path, kind, companyDomain string, out io.Writer) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("kind", kind)
	if companyDomain != "" {
		form.WriteField("company_domain", companyDomain)
	}
	part, err := form.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return err
	}
	part.Write(content)
	if err := form.Close(); err != nil {
		return err
	}
	var uploaded struct {
		Artifact store.Artifact `json:"artifact"`
		Created  bool           `json:"created"`
	}
	if err := callRESTWithContentType(ctx, config, http.MethodPost, "/v1/artifacts", &body, form.FormDataContentType(), &uploaded); err != nil {
		return err
	}
	if uploaded.Created {
		fmt.Fprintf(out, "Attached %s (%s, %s). Artifact id: %s\n", uploaded.Artifact.Name, uploaded.Artifact.Kind, formatSize(uploaded.Artifact.Size), uploaded.Artifact.ID)
	} else {
		fmt.Fprintf(out, "Already attached as %s. Artifact id: %s\n", uploaded.Artifact.Name, uploaded.Artifact.ID)
	}
	return nil
}

// listFiles prints the owner's files, or a company's.
func listFiles(ctx context.Context, config cliConfig, companyDomain string, out io.Writer) error {
	path := "/v1/artifacts"
	if companyDomain != "" {
		path += "?" + url.Values{"company_domain": {companyDomain}}.Encode()
	}
	var listed struct {
		Artifacts []store.Artifact `json:"artifacts"`
	}
	if err := getJSON(ctx, config, path, &listed); err != nil {
		return err
	}
	if len(listed.Artifacts) == 0 {
		fmt.Fprintln(out, "No files attached.")
		return nil
	}
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tKIND\tNAME\tSIZE\tADDED")
	for _, artifact := range listed.Artifacts {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", artifact.ID, artifact.Kind, artifact.Name, formatSize(artifact.Size), artifact.CreatedAt.Format("2006-01-02"))
	}
	return table.Flush()
}

// deleteFile removes an attached file.
func deleteFile(ctx context.Context, config cliConfig, rawID string, out io.Writer) error {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return fmt.Errorf("%q is not an artifact id", rawID)
	}
	if err := callRESTWithContentType(ctx, config, http.MethodDelete, "/v1/artifacts/"+id.String(), nil, "", nil); err != nil {
		return err
	}
	fmt.Fprintln(out, "Deleted.")
	return nil
}

func formatSize(bytes int) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
