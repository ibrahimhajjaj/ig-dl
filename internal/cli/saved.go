package cli

import (
	"github.com/ibrhajjaj/ig-dl/internal/core"
	"github.com/spf13/cobra"
)

func newSavedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "saved",
		Short: "Download your saved collection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			res, err := core.DownloadSaved(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return emit(res)
		},
	}
}
