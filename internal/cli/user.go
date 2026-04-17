package cli

import (
	"github.com/ibrahimhajjaj/ig-dl/internal/core"
	"github.com/spf13/cobra"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user <handle>",
		Short: "Download all content for a profile (posts, reels, stories, highlights)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			res, err := core.DownloadUser(cmd.Context(), args[0], flagInclude, opts)
			if err != nil {
				return err
			}
			return emit(res)
		},
	}
	cmd.Flags().StringSliceVar(&flagInclude, "include", nil, "limit stages: posts,reels,stories,highlights (default all)")
	return cmd
}
