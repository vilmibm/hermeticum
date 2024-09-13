package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vilmibm/hermeticum/server"
)

func init() {
	serveCmd.Flags().IntP("port", "p", 6666, "host port")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use: "serve",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		opts := server.ServeOpts{
			Port: port,
		}
		return server.Serve(opts)
	},
}
