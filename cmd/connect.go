package cmd

import (
	"github.com/spf13/cobra"
	"github.com/vilmibm/hermeticum/client"
)

func init() {
	rootCmd.AddCommand(connectCmd)
}

var connectCmd = &cobra.Command{
	Use: "connect",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := client.ConnectOpts{}
		return client.Connect(opts)
	},
}
