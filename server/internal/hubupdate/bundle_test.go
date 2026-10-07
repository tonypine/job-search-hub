package hubupdate

import (
	"os"
	"path/filepath"
	"testing"
)

// writeBundle makes a bundle of version at app, as make-app.sh would, with
// an executable hub-server.
func writeBundle(t *testing.T, app, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Helpers", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>JobSearchHub</string>
	<key>NSAppTransportSecurity</key>
	<dict>
		<key>CFBundleShortVersionString</key>
		<string>not this one</string>
	</dict>
	<key>CFBundleShortVersionString</key>
	<string>` + version + `</string>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CommandPath(app, "hub-server"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBundleVersionReadsTheTopLevelKey(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Job Search Hub.app")
	writeBundle(t, app, "0.1.252")
	if version, err := BundleVersion(app); err != nil || version != "0.1.252" {
		t.Fatalf("version = %q, %v", version, err)
	}
	if _, err := BundleVersion(filepath.Join(t.TempDir(), "missing.app")); err == nil {
		t.Fatal("a missing bundle has a version")
	}
}

func TestSwapExchangesTwoBundlesAtOnce(t *testing.T) {
	folder := t.TempDir()
	installed, downloaded := filepath.Join(folder, "installed.app"), filepath.Join(folder, "downloaded.app")
	writeBundle(t, installed, "0.1.247")
	writeBundle(t, downloaded, "0.1.252")
	if err := swap(downloaded, installed); err != nil {
		t.Fatal(err)
	}
	if !isVersion(installed, "0.1.252") || !isVersion(downloaded, "0.1.247") {
		t.Fatal("the bundles weren't exchanged")
	}
}
