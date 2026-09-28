// Command hub works with the job-search hub from the terminal.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const usage = `usage:
  hub watch-list              list the watched companies
  hub company show <domain>   print a company's dossier
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	config, err := readConfig(os.Getenv, home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	switch {
	case len(args) == 1 && args[0] == "watch-list":
		err = showWatchList(ctx, config, stdout)
	case len(args) == 3 && args[0] == "company" && args[1] == "show":
		err = showCompany(ctx, config, args[2], stdout)
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func showWatchList(ctx context.Context, config cliConfig, out io.Writer) error {
	session, err := connectToHub(ctx, config)
	if err != nil {
		return err
	}
	defer session.Close()

	listed, err := callTool[struct {
		Companies []store.WatchedCompany `json:"companies"`
	}](ctx, session, "list_watch_list", map[string]any{})
	if err != nil {
		return err
	}
	printWatchList(out, listed.Companies)
	return nil
}

func showCompany(ctx context.Context, config cliConfig, domain string, out io.Writer) error {
	session, err := connectToHub(ctx, config)
	if err != nil {
		return err
	}
	defer session.Close()

	dossier, err := callTool[store.CompanyDossier](ctx, session, "get_company", map[string]any{"domain": domain})
	if err != nil {
		if err.Error() == store.ErrCompanyNotFound.Error() {
			return errors.New("no company is stored under " + domain)
		}
		return err
	}
	printDossier(out, dossier)
	return nil
}
