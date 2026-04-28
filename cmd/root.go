package cmd

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ktrysmt/gh-rv/internal/api"
	"github.com/ktrysmt/gh-rv/internal/tui"
)

var (
	fixturePath   string
	simulateError string
	slowLoad      time.Duration
	diffHeight    int

	// Set via -ldflags at release time (see .goreleaser.yaml).
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "gh-rv [pr-number-or-url]",
	Short:   "PR review TUI for the gh CLI",
	Version: versionString(),
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		var (
			client api.Client
			err    error
		)
		switch {
		case simulateError != "":
			client = api.NewErrorClient(simulateError)
		case fixturePath != "":
			client, err = api.NewFixtureClient(fixturePath)
		default:
			client, err = api.NewGHClient()
		}
		if err != nil {
			return err
		}
		if slowLoad > 0 {
			client = api.WithSlowLoad(client, slowLoad)
		}

		ref, err := api.ParseTargetArg(ctx, client, args)
		if err != nil {
			return err
		}

		m := tui.NewModel(client, ref)
		if diffHeight > 0 {
			m.SetDiffHeight(diffHeight)
		}
		prog := tea.NewProgram(m, tea.WithAltScreen())
		final, err := prog.Run()
		if err != nil {
			return err
		}
		if fm, ok := final.(tui.Model); ok && fm.Err() != nil {
			return fm.Err()
		}
		return nil
	},
}

func init() {
	rootCmd.Flags().StringVar(&fixturePath, "fixture", "", "load PR data from JSON fixture instead of the gh API (testing)")
	rootCmd.Flags().StringVar(&simulateError, "simulate-error", "", "inject a simulated error: unauth | not_found | rate_limit (testing)")
	rootCmd.Flags().DurationVar(&slowLoad, "slow-load", 0, "inject per-call delay into the loader (testing)")
	rootCmd.Flags().IntVar(&diffHeight, "diff-height", 0, "force the Diff viewport height (testing)")
	_ = rootCmd.Flags().MarkHidden("simulate-error")
	_ = rootCmd.Flags().MarkHidden("slow-load")
	_ = rootCmd.Flags().MarkHidden("diff-height")
}

func Execute() error { return rootCmd.Execute() }

func versionString() string {
	return version + " (" + commit + ", " + date + ")"
}
