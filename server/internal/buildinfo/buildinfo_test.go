package buildinfo

import (
	"strings"
	"testing"
)

func TestAStampedBuildIsARelease(t *testing.T) {
	defer stamp("0.1.252", "4c1e8a9f00d1b2c3")()

	if Version() != "0.1.252" || Commit() != "4c1e8a9f00d1b2c3" || !IsRelease() {
		t.Fatalf("version %q, commit %q, release %v", Version(), Commit(), IsRelease())
	}
}

func TestAnUnstampedBuildIsADevVersionOfItsCommit(t *testing.T) {
	defer stamp("", "4c1e8a9f00d1b2c3")()

	if Version() != "0.1.0-dev.4c1e8a9" || IsRelease() {
		t.Fatalf("version %q, release %v", Version(), IsRelease())
	}
}

func TestABuildWithNoCommitIsStillADevVersion(t *testing.T) {
	defer stamp("", "")()

	// go test records no vcs.revision, so the commit is unknown.
	if !strings.HasPrefix(Version(), "0.1.0-dev") || IsRelease() {
		t.Fatalf("version %q, release %v", Version(), IsRelease())
	}
}

func stamp(newVersion, newCommit string) func() {
	oldVersion, oldCommit := version, commit
	version, commit = newVersion, newCommit
	return func() { version, commit = oldVersion, oldCommit }
}
