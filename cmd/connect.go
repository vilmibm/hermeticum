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
	Run: func(cmd *cobra.Command, args []string) {
		state, err := client.New()
		if err != nil {
			panic(err)
		}
		defer state.Close()
	loop:
		for {
			select {
			case <-client.Quit:
				break loop
			}
		}
	},
}
