package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/ktrysmt/gh-reva/internal/api"
	"github.com/ktrysmt/gh-reva/internal/config"
	"github.com/ktrysmt/gh-reva/internal/theme"
	"github.com/ktrysmt/gh-reva/internal/tui"
)

var (
	fixturePath   string
	simulateError string
	slowLoad      time.Duration
	diffHeight    int
	themeName     string
	noColor       bool
	listThemes    bool
	configPath    string

	// Set via -ldflags at release time (see .goreleaser.yaml).
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "gh-reva [pr-number-or-url]",
	Short:   "PR review TUI for the gh CLI",
	Version: versionString(),
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// --list-themes is a self-contained query; emit names and return
		// before touching the API or the renderer.
		if listThemes {
			out := cmd.OutOrStdout()
			for _, n := range theme.ListThemes() {
				fmt.Fprintln(out, n)
			}
			return nil
		}

		name := themeName
		if name == "" {
			name = os.Getenv("GH_REVA_THEME")
		}
		th, err := theme.Resolve(name)
		if err != nil {
			return err
		}

		// lipgloss v1 has no per-program color profile option; the
		// default renderer is the one bubbletea will see, so we lock it
		// before tea.NewProgram. EnvNoColor honors NO_COLOR / CLICOLOR
		// for users who set the standard env vars.
		if noColor || termenv.EnvNoColor() {
			lipgloss.SetColorProfile(termenv.Ascii)
		}
		// gh-reva ships a single dark palette; lock background detection so
		// adaptive components used by future deps render predictably.
		lipgloss.SetHasDarkBackground(true)

		// Load reva.toml early so a typo in --config or invalid TOML
		// surfaces before any network round-trip. --config wins; the
		// implicit XDG ladder picks the first existing default.
		// ResolvePath returns "" when no candidate exists, which Load
		// turns into an empty Config so the rest of the flow runs as if
		// no config were ever set.
		cfgPath := config.ResolvePath(configPath)
		cfg, cfgErr := config.Load(cfgPath)
		if cfgErr != nil {
			return cfgErr
		}

		ctx := context.Background()

		var client api.Client
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
		ref, err := api.ParseTargetArg(ctx, client, args)
		if err != nil {
			return err
		}

		// Pre-flight probe: hit GetPR once before opening the TUI so API
		// failures (auth / 404 / rate limit / network) surface to stderr
		// instead of vanishing behind the alt-screen as a brief splash
		// flash. Also covers ParseTargetArg's recovery branch
		// (resolve.go:36-44) which silently swallows ResolveCurrentBranchPR
		// errors when the user supplies an explicit PR arg — without the
		// probe, an auth or rate-limit failure there would let control
		// reach `tea.NewProgram().Run()`, where the loader's first stage
		// fires the same error inside the alt-screen and the user only
		// sees the program quit. The 10s timeout matches the loader's
		// implicit budget for the first stage.
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if _, err := client.GetPR(probeCtx, ref.Owner, ref.Repo, ref.Number); err != nil {
			return err
		}

		// Apply --slow-load AFTER the probe so the splash is visible
		// during preview. The probe re-runs the same call WithSlowLoad
		// would intercept; if the wrapper sat above the probe (the
		// previous order), the user would see a black terminal for the
		// full slow-load duration BEFORE the TUI launches — defeating
		// the flag's stated purpose ("observe the splash / spinner").
		// fixtureClient.wait() ignores ctx, so the 10s probe timeout
		// would not have saved us either.
		if slowLoad > 0 {
			client = api.WithSlowLoad(client, slowLoad)
		}

		m := tui.NewModel(client, ref)
		m.SetTheme(th)
		m.SetVersion(version)
		m.SetSyntaxExtensions(cfg.Syntax.Extensions)
		m.SetCommentsWidthPercent(cfg.Layout.CommentsWidthPercent)
		if diffHeight > 0 {
			m.SetDiffHeight(diffHeight)
		}
		// Cell-motion mouse capture lets the TUI receive click and
		// wheel events without intercepting per-pixel motion (cheaper
		// than WithMouseAllMotion). Standard terminals (macOS Terminal,
		// iTerm2, Alacritty, Ghostty, kitty) still let users hold
		// Shift to bypass the mouse-tracking grab and fall back to
		// terminal-side text selection — so copy/paste still works
		// without an explicit toggle.
		prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
	rootCmd.Flags().StringVar(&themeName, "theme", "", "color theme name (default: gruvbox; see --list-themes)")
	rootCmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output (also honors NO_COLOR / CLICOLOR)")
	rootCmd.Flags().BoolVar(&listThemes, "list-themes", false, "print every available theme name and exit")
	rootCmd.Flags().StringVar(&configPath, "config", "", "path to reva.toml (default: $XDG_CONFIG_HOME/reva.toml or $HOME/.config/reva.toml)")
	_ = rootCmd.Flags().MarkHidden("simulate-error")
	_ = rootCmd.Flags().MarkHidden("slow-load")
	_ = rootCmd.Flags().MarkHidden("diff-height")
	// Suppress cobra's built-in error / usage printing so the wrapper in
	// main.go controls the format (single colored line via theme.ErrorText).
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
}

func Execute() error { return rootCmd.Execute() }

// PrintError writes err to stderr coloured by theme.ErrorText. The default
// (gruvbox) palette is used because errors can fire before --theme is
// resolved or precisely because --theme failed; consistency over fidelity.
// In a non-TTY (lipgloss falls back to Ascii profile), the SGR wrapping is
// a no-op and the output remains plain text — substring matchers in the
// e2e suite continue to work.
func PrintError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if th, terr := theme.Resolve(""); terr == nil && th != nil && th.ErrorText != "" {
		msg = lipgloss.NewStyle().Foreground(th.ErrorText).Render(msg)
	}
	fmt.Fprintln(os.Stderr, msg)
}

func versionString() string {
	return version + " (" + commit + ", " + date + ")"
}
