package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) conferencesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conferences",
		Short: "List available CVF conferences",
		Long: `List all conferences available in the CVF open access repository.

Examples:
  cvf conferences
  cvf conferences -f json
  cvf conferences --fields name,year,location`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.progressf("fetching conference list...")
			confs, err := a.client.Conferences(cmd.Context())
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(confs, len(confs))
		},
	}
	return cmd
}
