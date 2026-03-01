package main

import (
	"github.com/spf13/cobra"

	"github.com/aronasorman/sling/internal/pipeline"
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Show a digest of beads that need attention",
	Args:  cobra.NoArgs,
	RunE:  runMail,
}

func init() {
	rootCmd.AddCommand(mailCmd)
}

func runMail(cmd *cobra.Command, args []string) error {
	return pipeline.Mail()
}
