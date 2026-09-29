// Command hub works with the job-search hub from the terminal.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

const usage = `usage:
  hub watch-list                    list the watched companies
  hub company show <domain>         print a company's dossier
  hub company add <name-or-url>     research a company with an agent and watch it
      [--found-via <note>] [--model <model>] [--effort <level>]
  hub company find-jobs <domain-or-id>
                                    find a company's open roles with an agent
      [--model <model>] [--effort <level>]
  hub recruiter reply <conversation-id>
      [--model <model>]             draft a message back to a recruiter who wrote before
  hub profile audit [--model <model>]
                                    audit the LinkedIn profile for recruiters searching
  hub attach <path> --kind resume|company_document|saved_page|other [--company <domain>]
                                    attach a file the agents read as context
  hub artifacts [--company <domain>]
                                    list your attached files, or a company's
  hub artifacts delete <id>         delete an attached file
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
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
	case len(args) >= 3 && args[0] == "company" && args[1] == "add":
		flags := flag.NewFlagSet("company add", flag.ContinueOnError)
		flags.SetOutput(stderr)
		var options triageOptions
		flags.StringVar(&options.model, "model", "", "the Claude model for the session")
		flags.StringVar(&options.effort, "effort", "", "the effort level for the session")
		flags.StringVar(&options.foundVia, "found-via", "", "how the company reached you, e.g. a referral")
		if flags.Parse(args[3:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = addCompany(ctx, config, args[2], options, stdout)
	case len(args) >= 3 && args[0] == "company" && args[1] == "find-jobs":
		flags := flag.NewFlagSet("company find-jobs", flag.ContinueOnError)
		flags.SetOutput(stderr)
		var options triageOptions
		flags.StringVar(&options.model, "model", "", "the Claude model for the session")
		flags.StringVar(&options.effort, "effort", "", "the effort level for the session")
		if flags.Parse(args[3:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = findCompanyJobs(ctx, config, args[2], options, stdout)
	case len(args) >= 3 && args[0] == "recruiter" && args[1] == "reply":
		flags := flag.NewFlagSet("recruiter reply", flag.ContinueOnError)
		flags.SetOutput(stderr)
		model := flags.String("model", "", "the Claude model for the draft")
		if flags.Parse(args[3:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = draftRecruiterReply(ctx, config, args[2], *model, stdout)
	case len(args) >= 2 && args[0] == "profile" && args[1] == "audit":
		flags := flag.NewFlagSet("profile audit", flag.ContinueOnError)
		flags.SetOutput(stderr)
		model := flags.String("model", "", "the Claude model for the audit")
		if flags.Parse(args[2:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = auditProfile(ctx, config, *model, stdout)
	case len(args) >= 2 && args[0] == "attach":
		flags := flag.NewFlagSet("attach", flag.ContinueOnError)
		flags.SetOutput(stderr)
		kind := flags.String("kind", "", "resume, company_document, saved_page or other")
		company := flags.String("company", "", "the domain of the company the file is about")
		if flags.Parse(args[2:]) != nil || flags.NArg() > 0 || *kind == "" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = attachFile(ctx, config, args[1], *kind, *company, stdout)
	case len(args) == 3 && args[0] == "artifacts" && args[1] == "delete":
		err = deleteFile(ctx, config, args[2], stdout)
	case len(args) >= 1 && args[0] == "artifacts":
		flags := flag.NewFlagSet("artifacts", flag.ContinueOnError)
		flags.SetOutput(stderr)
		company := flags.String("company", "", "the domain of the company whose files to list")
		if flags.Parse(args[1:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = listFiles(ctx, config, *company, stdout)
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
