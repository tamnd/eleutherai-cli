package cli

import (
	"github.com/spf13/cobra"
)

func newLatestCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "latest",
		Short: "List the most recent blog posts",
		Long: `latest fetches the blog RSS feed and prints the most recent posts.

Examples:
  eai latest
  eai latest -n 5
  eai latest -f json
  eai latest --fields title,url -f url`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			posts, err := app.client.Latest(cmd.Context(), app.limit)
			if err != nil {
				return err
			}
			if len(posts) == 0 {
				return noResults("no posts found")
			}
			return app.render(posts)
		},
	}
}
