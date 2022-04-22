package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

type CobraFunc func(cmd *cobra.Command, args []string) error

type Middleware func(CobraFunc) CobraFunc

var nop CobraFunc = func(*cobra.Command, []string) error { return nil }

func Errorf(message string, args ...any) CobraFunc {
	return func(*cobra.Command, []string) error {
		return fmt.Errorf(message, args...)
	}
}

func Use(cmd *cobra.Command, mw Middleware) {
	var apply func(*cobra.Command)
	apply = func(cmd *cobra.Command) {
		run := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			return mw(run)(cmd, args)
		}
		for _, cmd := range cmd.Commands() {
			apply(cmd)
		}
	}
	apply(cmd)
}

func PreUse(cmd *cobra.Command, mw Middleware) {
	var apply func(*cobra.Command)
	apply = func(cmd *cobra.Command) {
		pre := cmd.PreRunE
		if pre == nil {
			pre = nop
		}
		cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
			return mw(pre)(cmd, args)
		}
		for _, cmd := range cmd.Commands() {
			apply(cmd)
		}
	}
	apply(cmd)
}

func PostUse(cmd *cobra.Command, mw Middleware) {
	var apply func(*cobra.Command)
	apply = func(cmd *cobra.Command) {
		post := cmd.PostRunE
		if post == nil {
			post = nop
		}
		cmd.PostRunE = func(cmd *cobra.Command, args []string) error {
			return mw(post)(cmd, args)
		}
		for _, cmd := range cmd.Commands() {
			apply(cmd)
		}
	}
	apply(cmd)
}
