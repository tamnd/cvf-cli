package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) papersCmd() *cobra.Command {
	var conference string
	var year int

	cmd := &cobra.Command{
		Use:   "papers",
		Short: "List papers from a CVF conference",
		Long: `List papers from a CVF open access conference.

Defaults to CVPR 2024 when no flags are given.

Examples:
  cvf papers
  cvf papers --conference CVPR --year 2024 --limit 20
  cvf papers --conference ICCV --year 2023 -f json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(20)
			a.progressf("fetching papers from %s %d...", conference, year)
			papers, err := a.client.Papers(cmd.Context(), conference, year, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(papers, len(papers))
		},
	}

	cmd.Flags().StringVarP(&conference, "conference", "c", "CVPR", "conference name (CVPR, ICCV, ECCV, WACV)")
	cmd.Flags().IntVarP(&year, "year", "y", 2024, "conference year")
	return cmd
}
