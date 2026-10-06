// Package hubclients reads which app makes a request, from the X-Hub-Client
// header the Mac app and the phone send with every one, and turns away the
// versions the server no longer serves.
package hubclients

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Header names the app and its version: macos/0.1.252 or android/0.1.252.
const Header = "X-Hub-Client"

// Platforms that send the header.
const (
	MacOS   = "macos"
	Android = "android"
)

// Client is the app a request came from.
type Client struct {
	Platform string
	Version  string
}

// Parse reads the header's value; false for a request without one, as from
// hub or an agent, or with one that isn't platform/version.
func Parse(header string) (Client, bool) {
	platform, version, found := strings.Cut(strings.TrimSpace(header), "/")
	if !found || platform == "" || version == "" || strings.ContainsAny(version, " /") {
		return Client{}, false
	}
	return Client{Platform: platform, Version: version}, true
}

// FromRequest is the app that made the request, if it said.
func FromRequest(r *http.Request) (Client, bool) {
	return Parse(r.Header.Get(Header))
}

// Gate answers 426 Upgrade Required, with a message the app shows, to an
// app older than its platform's minimum release. Health and version are
// always answered, so an app can still tell which hub it reaches.
type Gate struct {
	// Minimums is the oldest release served per platform, such as
	// {"android": "0.1.200"}; a platform without one is always served.
	Minimums map[string]string
	// ServerVersion is the hub's own version, for the message.
	ServerVersion string
}

// Wrap puts the gate in front of next.
func (gate Gate) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if message, refused := gate.refusal(r); refused {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUpgradeRequired)
			_ = json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{message})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (gate Gate) refusal(r *http.Request) (string, bool) {
	if r.URL.Path == "/v1/health" || r.URL.Path == "/v1/version" {
		return "", false
	}
	client, found := FromRequest(r)
	if !found {
		return "", false
	}
	minimum, found := gate.Minimums[client.Platform]
	if !found || !isOlder(client.Version, minimum) {
		return "", false
	}
	return fmt.Sprintf("This app (%s) is too old for the hub, which runs %s. Update the app to %s or later.", client.Version, gate.ServerVersion, minimum), true
}

// isOlder says whether release version comes before minimum. A dev build,
// or a version that isn't major.minor.patch, is never turned away.
func isOlder(version, minimum string) bool {
	have, ok := parseRelease(version)
	if !ok {
		return false
	}
	want, ok := parseRelease(minimum)
	if !ok {
		return false
	}
	for i := range have {
		if have[i] != want[i] {
			return have[i] < want[i]
		}
	}
	return false
}

func parseRelease(version string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(version, ".")
	if len(fields) != len(parts) {
		return parts, false
	}
	for i, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return parts, false
		}
		parts[i] = number
	}
	return parts, true
}
