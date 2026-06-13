// Package cli builds the eai command tree on top of the eleutherai library.
package cli

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/eleutherai-cli/eleutherai"
)

// Build metadata, injected via -ldflags by the Makefile/goreleaser.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// App holds the shared client and global flag values for a command run.
type App struct {
	cfg      eleutherai.Config
	client   *eleutherai.Client
	format   string
	fields   []string
	noHeader bool
	template string
	limit    int
}

// Root builds the root command and its whole subtree.
func Root() *cobra.Command {
	app := &App{cfg: eleutherai.DefaultConfig()}

	root := &cobra.Command{
		Use:   "eai",
		Short: "Browse EleutherAI research blog posts",
		Long: `eai reads the EleutherAI research blog (blog.eleuther.ai) and
returns structured records as table, JSON, JSONL, CSV, TSV, or URLs.

Quick start:
  eai latest -n 5           list the five most recent posts
  eai search "reward"       find posts about reward hacking`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			app.client = eleutherai.NewClient(app.cfg)
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&app.format, "format", "f", "", "output: table|json|jsonl|csv|tsv|url|raw (default: table on TTY, jsonl when piped)")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv output")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.IntVarP(&app.limit, "limit", "n", 10, "max results (0 = all)")
	pf.StringVar(&app.cfg.BaseURL, "base-url", eleutherai.DefaultBaseURL, "blog base URL")
	pf.IntVar(&app.cfg.Retries, "retries", 5, "retry attempts on 429/5xx")

	root.AddCommand(
		newLatestCmd(app),
		newSearchCmd(app),
		newVersionCmd(),
	)
	return root
}

// render writes records using the resolved global flags.
func (a *App) render(records any) error {
	format := a.format
	if format == "" {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			format = string(FormatTable)
		} else {
			format = string(FormatJSONL)
		}
	}
	r := NewRenderer(os.Stdout, Format(format), a.fields, a.noHeader, a.template)
	return r.Render(records)
}

// renderOrEmpty renders records, returning exit code 3 when none matched.
func (a *App) renderOrEmpty(records any, n int) error {
	if err := a.render(records); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no results found")
	}
	return nil
}
