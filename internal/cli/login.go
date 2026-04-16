package cli

import (
	"fmt"

	"github.com/ibrhajjaj/ig-dl/internal/core"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Capture or import a session from a running Chrome",
		Long: `Attach to a running Chrome on the configured debug port and capture the
current Instagram session (cookies + rotating headers). Or use --import to
load a session.json produced by the companion extension.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			if flagImport != "" {
				if err := core.ImportSession(opts, flagImport); err != nil {
					return fmt.Errorf("import: %w", err)
				}
				fmt.Println("session imported")
				return nil
			}
			source, err := core.Login(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Printf("session captured from %s\n", source)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagImport, "import", "", "import session.json exported by the companion extension")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the cached session and cookies files",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			if err := core.Logout(opts); err != nil {
				return err
			}
			fmt.Println("session cleared")
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether a session is cached and how old it is",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			authed, age, source, err := core.SessionStatus(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if flagJSON {
				return emitJSON(map[string]any{
					"authed":      authed,
					"source":      source,
					"age_seconds": age,
				})
			}
			if !authed {
				fmt.Println("not authed — run `ig-dl login`")
				return nil
			}
			fmt.Printf("authed (source=%s, age=%.0fs)\n", source, age)
			return nil
		},
	}
}
