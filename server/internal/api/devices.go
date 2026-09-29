package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type devicesResponse struct {
	Devices []store.Device `json:"devices"`
}

type pairDeviceRequest struct {
	Name string `json:"name"`
}

// pairDeviceResponse carries the device's token, shown this once.
type pairDeviceResponse struct {
	Device store.Device `json:"device"`
	Token  string       `json:"token"`
}

// RegisterDeviceRoutes adds the routes for the phones paired with the hub.
// Pairing takes the owner's own token, from the Mac; a phone can list the
// paired devices and revoke one, itself included.
func RegisterDeviceRoutes(routes *http.ServeMux, hub *store.Store, requireOwner func(http.Handler) http.Handler) {
	owner := store.Actor{Kind: store.ActorOwner}
	routes.Handle("GET /v1/devices", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		devices, err := hub.ListDevices(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, devicesResponse{Devices: devices})
	})))

	routes.Handle("POST /v1/devices", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tokens.IsOwnersOwnToken(r.Context()) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "pair a phone from the Mac"})
			return
		}
		var request pairDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the body must be JSON: " + err.Error()})
			return
		}
		token, tokenHash := tokens.NewDeviceToken()
		device, err := hub.CreateDevice(r.Context(), owner, request.Name, tokenHash)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, pairDeviceResponse{Device: device, Token: token})
	})))

	routes.Handle("DELETE /v1/devices/{id}", requireOwner(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		device, err := hub.RevokeDevice(r.Context(), owner, id)
		switch {
		case errors.Is(err, store.ErrDeviceNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusOK, device)
		}
	})))
}
