package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vilmibm/hermeticum/server/db"
)

func init() {
	rootCmd.AddCommand(resetCmd)
}

var resetCmd = &cobra.Command{
	Use: "reset",
	RunE: func(cmd *cobra.Command, args []string) error {
		hdb, err := db.NewDB()
		if err != nil {
			return err
		}

		opts := db.ResetOpts{
			DB: hdb,
		}
		return db.Reset(opts)
	},
}
