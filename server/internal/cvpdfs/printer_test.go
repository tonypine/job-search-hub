package cvpdfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
)

var owner = store.Actor{Kind: store.ActorOwner}

// writeFakePrintCommand writes a print command that copies its HTML into the
// PDF after a %PDF header, so a test can tell prints apart.
func writeFakePrintCommand(t *testing.T) string {
	t.Helper()
	command := filepath.Join(t.TempDir(), "hub-cvprint")
	script := "#!/bin/sh\n{ printf '%%PDF-fake\\n'; cat \"$1\"; } > \"$2\"\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return command
}

func TestATailoredCVPrintsToItsJobsFolderUnderTheOwnersName(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	company, _, _ := hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme: Labs", Domain: "acme.example"})
	job, _, _ := hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Front-End / UI Engineer", URL: "https://acme.example/1"})
	content := resume.Resume{Basics: resume.Basics{Name: "Ada  Lovelace", Label: "Front-End Engineer"}}
	cv, err := hub.SaveTailoredCV(ctx, owner, job.ID, content, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	printer := NewPrinter(hub, writeFakePrintCommand(t), folder)

	printed, err := printer.PrintCV(ctx, owner, cv.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(folder, "Acme Labs - Front-End - UI Engineer", "Ada_Lovelace_CV.pdf")
	if printed.PDFPath != want || !printed.HasPDF {
		t.Fatalf("printed to %q (has pdf %v), want %q", printed.PDFPath, printed.HasPDF, want)
	}
	first, _ := os.ReadFile(want)
	if !strings.HasPrefix(string(first), "%PDF") || !strings.Contains(string(first), "Front-End Engineer") {
		t.Fatalf("the file holds %q", first[:min(len(first), 80)])
	}
	if stored, _ := hub.GetCVPDF(ctx, cv.ID); string(stored) != string(first) {
		t.Error("the hub keeps a different PDF than the file")
	}

	content.Basics.Label = "UI Engineer"
	if _, err := hub.SaveTailoredCV(ctx, owner, job.ID, content, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if edited, _ := hub.GetCV(ctx, cv.ID); edited.HasPDF || edited.PDFPath != want {
		t.Errorf("after an edit: has pdf %v at %q, want no PDF but the path kept", edited.HasPDF, edited.PDFPath)
	}
	if _, err := printer.PrintCV(ctx, owner, cv.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(want); !strings.Contains(string(again), "UI Engineer") || strings.Contains(string(again), "Front-End Engineer") {
		t.Error("printing again didn't replace the file")
	}
}

func TestTheBaseCVPrintsToItsOwnFolderAndAMissingCommandFails(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	ctx := context.Background()
	base, err := hub.SaveBaseCV(ctx, owner, resume.Resume{Basics: resume.Basics{Name: "Ada Lovelace"}})
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if printed, err := NewPrinter(hub, writeFakePrintCommand(t), folder).PrintCV(ctx, owner, base.ID); err != nil ||
		printed.PDFPath != filepath.Join(folder, "Base CV", "Ada_Lovelace_CV.pdf") {
		t.Fatalf("base printed to %q, %v", printed.PDFPath, err)
	}
	if _, err := NewPrinter(hub, filepath.Join(t.TempDir(), "missing"), folder).PrintCV(ctx, owner, base.ID); err == nil || !strings.Contains(err.Error(), "print the CV") {
		t.Errorf("a missing command: %v", err)
	}
}
