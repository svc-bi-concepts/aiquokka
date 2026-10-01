package cmd

import (
	"github.com/McKean/aiquokka/internal/zai"
	"github.com/spf13/cobra"
)

func newZaiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "zai",
		Short: "Z.ai GLM Coding Plan windows, usage bundles and cash balance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(zai.Fetch)
		},
	}
}
