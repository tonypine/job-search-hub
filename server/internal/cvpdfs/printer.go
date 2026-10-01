// Package cvpdfs prints CVs to PDF files on this machine, through the
// hub-cvprint command (WebKit, offscreen), so a CV can be opened, shown in
// Finder and attached to an application form.
package cvpdfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/resume"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	// printTimeout bounds one print; the command itself gives up at 60 s.
	printTimeout = 90 * time.Second
	// maximumFolderNameLength keeps a job's folder name readable.
	maximumFolderNameLength = 120
	baseCVFolderName        = "Base CV"
)

// ErrNotTailored is a print asked of a CV with no job and no base kind.
var ErrNotTailored = errors.New("only the base CV and tailored CVs print")

// Printer prints CVs with the command at Command into Folder, one folder per
// job, each PDF named after the owner as their own CV is.
type Printer struct {
	hub     *store.Store
	Command string
	Folder  string
}

func NewPrinter(hub *store.Store, command, folder string) *Printer {
	return &Printer{hub: hub, Command: command, Folder: folder}
}

// PrintCV prints the CV to its file, replacing an earlier print, and keeps
// the PDF and its path with the CV.
func (printer *Printer) PrintCV(ctx context.Context, actor store.Actor, cvID uuid.UUID) (store.CV, error) {
	cv, err := printer.hub.GetCV(ctx, cvID)
	if err != nil {
		return store.CV{}, err
	}
	folder, err := printer.getCVFolder(ctx, cv)
	if err != nil {
		return store.CV{}, err
	}
	html, err := resume.RenderHTML(cv.Content)
	if err != nil {
		return store.CV{}, err
	}
	pdf, err := printer.printHTML(ctx, html)
	if err != nil {
		return store.CV{}, err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return store.CV{}, err
	}
	path := filepath.Join(folder, buildFileName(cv.Content.Basics.Name))
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return store.CV{}, err
	}
	if err := printer.hub.SaveCVPDF(ctx, actor, cv.ID, pdf, path); err != nil {
		return store.CV{}, err
	}
	return printer.hub.GetCV(ctx, cv.ID)
}

// getCVFolder is the CV's folder: its job's company and title for a tailored
// CV, or one for the base CV.
func (printer *Printer) getCVFolder(ctx context.Context, cv store.CV) (string, error) {
	if cv.JobID == nil {
		if cv.Kind != store.CVKindBase {
			return "", ErrNotTailored
		}
		return filepath.Join(printer.Folder, baseCVFolderName), nil
	}
	details, err := printer.hub.GetJobDetails(ctx, *cv.JobID)
	if err != nil {
		return "", err
	}
	name := details.Job.Title
	if details.CompanyName != nil && *details.CompanyName != "" {
		name = *details.CompanyName + " - " + name
	}
	return filepath.Join(printer.Folder, buildFolderName(name)), nil
}

// printHTML runs the print command on the HTML and returns the PDF.
func (printer *Printer) printHTML(ctx context.Context, html []byte) ([]byte, error) {
	work, err := os.MkdirTemp("", "hub-cvprint-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	input, output := filepath.Join(work, "cv.html"), filepath.Join(work, "cv.pdf")
	if err := os.WriteFile(input, html, 0o600); err != nil {
		return nil, err
	}
	printCtx, cancel := context.WithTimeout(ctx, printTimeout)
	defer cancel()
	if result, err := exec.CommandContext(printCtx, printer.Command, input, output).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("print the CV: %w: %s", err, strings.TrimSpace(string(result)))
	}
	return os.ReadFile(output)
}

// buildFolderName makes a job's name safe as one folder name: slashes become
// dashes, colons spaces, and there's no leading dot or excess length.
func buildFolderName(name string) string {
	cleaned := strings.Map(func(character rune) rune {
		switch {
		case character == '/' || character == '\\':
			return '-'
		case character == ':':
			return ' '
		case character < ' ':
			return -1
		default:
			return character
		}
	}, name)
	cleaned = strings.Trim(strings.Join(strings.Fields(cleaned), " "), ". ")
	if len([]rune(cleaned)) > maximumFolderNameLength {
		cleaned = strings.TrimSpace(string([]rune(cleaned)[:maximumFolderNameLength]))
	}
	if cleaned == "" {
		return "Untitled job"
	}
	return cleaned
}

// buildFileName names the PDF as the owner's own CV is named, such as
// "Tony_Pine_CV.pdf", which is what a recruiter sees.
func buildFileName(ownerName string) string {
	words := strings.Fields(buildFolderName(ownerName))
	if len(words) == 0 || ownerName == "" {
		return "CV.pdf"
	}
	return strings.Join(words, "_") + "_CV.pdf"
}
