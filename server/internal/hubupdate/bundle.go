package hubupdate

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BundleVersion is the app bundle's CFBundleShortVersionString, read from
// its Contents/Info.plist, which make-app.sh writes as XML.
func BundleVersion(app string) (string, error) {
	file, err := os.Open(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	var key string
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s names no version", app)
		}
		if err != nil {
			return "", fmt.Errorf("read %s's Info.plist: %w", app, err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			// The top dict's keys are at depth 3: plist, dict, key.
			if depth != 3 || (element.Name.Local != "key" && element.Name.Local != "string") {
				continue
			}
			var text string
			if err := decoder.DecodeElement(&text, &element); err != nil {
				return "", err
			}
			depth--
			if element.Name.Local == "key" {
				key = text
			} else if key == "CFBundleShortVersionString" {
				return text, nil
			}
		case xml.EndElement:
			depth--
		}
	}
}

// CommandPath is the command name in the bundle's Helpers/bin.
func CommandPath(app, name string) string {
	return filepath.Join(app, "Contents", "Helpers", "bin", name)
}

// isVersion reports whether app is a bundle of version.
func isVersion(app, version string) bool {
	found, err := BundleVersion(app)
	return err == nil && found == version
}
