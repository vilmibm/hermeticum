package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vilmibm/hermeticum/server"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use: "serve",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := server.ServeOpts{}
		return server.Serve(opts)
	},
}
