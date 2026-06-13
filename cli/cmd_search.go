package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) searchCmd() *cobra.Command {
	var conference string
	var year int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search papers by title or author",
		Long: `Search papers from a CVF conference by title or author name.

The query is matched case-insensitively against each paper's title and author list.

Examples:
  cvf search "object detection"
  cvf search --conference ICCV --year 2023 "transformer" --limit 10
  cvf search "LeCun" --conference CVPR --year 2023`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			n := a.effectiveLimit(0)
			a.progressf("searching %s %d for %q...", conference, year, query)
			papers, err := a.client.Search(cmd.Context(), query, conference, year, n)
			if err != nil {
				return mapFetchErr(err)
			}
			if len(papers) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no papers matched %q\n", query)
				return codeError(exitNoData, nil)
			}
			return a.renderOrEmpty(papers, len(papers))
		},
	}

	cmd.Flags().StringVarP(&conference, "conference", "c", "CVPR", "conference name (CVPR, ICCV, ECCV, WACV)")
	cmd.Flags().IntVarP(&year, "year", "y", 2024, "conference year")
	return cmd
}
