package postgresprocess

import (
	"os/exec"
	"strings"
	"testing"
)

func TestAFolderExcludedFromBackupsIsExcludedInTimeMachine(t *testing.T) {
	dir := t.TempDir()
	if err := excludeFromBackups(dir); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/usr/bin/tmutil", "isexcluded", dir).Output()
	if err != nil || !strings.Contains(string(output), "[Excluded]") {
		t.Fatalf("tmutil isexcluded: %s, %v", output, err)
	}
}
