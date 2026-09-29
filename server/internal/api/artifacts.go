package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// artifactUploadOverhead is room for the form around the file itself.
const artifactUploadOverhead = 1 << 20

type artifactUploadResponse struct {
	Artifact store.Artifact `json:"artifact"`
	Created  bool           `json:"created"`
}

type artifactsResponse struct {
	Artifacts []store.Artifact `json:"artifacts"`
}

// RegisterArtifactRoutes adds the owner-only routes for uploading, listing
// and deleting the files agents read as context.
func RegisterArtifactRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	// POST /v1/artifacts takes a multipart form: the file, its kind, and the
	// company it is about as company_id or company_domain, or neither for a
	// file about the owner.
	routes.Handle("POST /v1/artifacts", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, store.MaximumArtifactSize+artifactUploadOverhead)
		file, header, err := r.FormFile("file")
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "files over 10 MB are refused"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "send the file as the multipart field \"file\""})
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, store.MaximumArtifactSize+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if len(content) > store.MaximumArtifactSize {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "files over 10 MB are refused"})
			return
		}
		companyID, status, err := findArtifactCompany(r, hub, r.FormValue("company_id"), r.FormValue("company_domain"))
		if err != nil {
			writeJSON(w, status, errorResponse{Error: err.Error()})
			return
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(content)
		}
		artifact, created, err := hub.SaveArtifact(r.Context(), store.Actor{Kind: store.ActorOwner}, store.NewArtifact{
			CompanyID: companyID, Kind: r.FormValue("kind"), Name: header.Filename, ContentType: contentType, Content: content,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		status = http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, artifactUploadResponse{Artifact: artifact, Created: created})
	})))

	routes.Handle("GET /v1/artifacts", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		companyID, status, err := findArtifactCompany(r, hub, query.Get("company_id"), query.Get("company_domain"))
		if err != nil {
			writeJSON(w, status, errorResponse{Error: err.Error()})
			return
		}
		artifacts, err := hub.ListArtifacts(r.Context(), companyID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, artifactsResponse{Artifacts: artifacts})
	})))

	routes.Handle("DELETE /v1/artifacts/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		err = hub.DeleteArtifact(r.Context(), store.Actor{Kind: store.ActorOwner}, id)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, store.ErrArtifactNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		}
	})))
}

// findArtifactCompany returns the company a file is about, from its id or
// its domain, or nil for the owner's own files.
func findArtifactCompany(r *http.Request, hub *store.Store, rawID, domain string) (*uuid.UUID, int, error) {
	switch {
	case rawID != "":
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("company_id must be a uuid")
		}
		if _, err := hub.GetCompany(r.Context(), id); err != nil {
			return nil, http.StatusNotFound, errors.New("no such company")
		}
		return &id, http.StatusOK, nil
	case domain != "":
		company, err := hub.GetCompanyByDomain(r.Context(), domain)
		if err != nil {
			return nil, http.StatusNotFound, errors.New("no company with the domain " + domain)
		}
		return &company.ID, http.StatusOK, nil
	default:
		return nil, http.StatusOK, nil
	}
}
