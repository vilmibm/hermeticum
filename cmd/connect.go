package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vilmibm/hermeticum/client"
)

func init() {
	connectCmd.Flags().IntP("port", "p", 6666, "host port")
	rootCmd.AddCommand(connectCmd)
}

var connectCmd = &cobra.Command{
	Use: "client",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		opts := client.ConnectOpts{
			Port: port,
		}
		return client.Connect(opts)
	},
}
