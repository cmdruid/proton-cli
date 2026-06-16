// Package cli wires the Cobra command tree. Command bodies are prepared by the
// pipeline (auth/unlock/resolve steps); this file owns the root command,
// global flags, exit-code plumbing and the leading-dash-ID workaround.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// version is overridden at release time via -ldflags -X. Cobra reads it at
// init when the rootCmd literal is evaluated.
var version = "dev"

type globalFlags struct {
	profile    string
	user       string
	password   string
	totp       string
	apiURL     string
	appVersion string
	output     string
	verbose    bool
	quiet      bool
	logLevel   string
	dryRun     bool
	fullIDs    bool
}

var gFlags globalFlags

var rootCmd = &cobra.Command{
	Use:           "proton-cli",
	Short:         "CLI for the Proton API",
	Long:          "An unofficial command-line tool for Proton (Mail, Drive, Calendar, Pass, Contacts). Handles SRP authentication and end-to-end encryption automatically.",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&gFlags.profile, "profile", "", "config profile to use (default: default)")
	pf.StringVar(&gFlags.user, "user", "", "Proton account email (env: PROTON_USER)")
	pf.StringVar(&gFlags.password, "password", "", "Account password (env: PROTON_PASSWORD)")
	pf.StringVar(&gFlags.totp, "totp", "", "TOTP 2FA code (env: PROTON_TOTP)")
	pf.StringVar(&gFlags.apiURL, "api-url", "", "API base URL (env: PROTON_API_URL)")
	pf.StringVar(&gFlags.appVersion, "app-version", "", "App version header (env: PROTON_APP_VERSION)")
	pf.StringVar(&gFlags.output, "output", "text", "Output format: text, json, yaml")
	pf.BoolVar(&gFlags.verbose, "verbose", false, "Enable debug logging")
	pf.BoolVar(&gFlags.quiet, "quiet", false, "Suppress non-essential stderr output")
	pf.StringVar(&gFlags.logLevel, "log-level", "", "Log level: debug, info, warn, error")
	pf.BoolVar(&gFlags.dryRun, "dry-run", false, "Preview mutations without applying them")
	pf.BoolVar(&gFlags.fullIDs, "full-ids", false, "Show full IDs in interactive output (default: shortened to 8 chars on TTY)")

	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		a, err := app.New(app.Options{
			Profile:    gFlags.profile,
			User:       gFlags.user,
			Password:   gFlags.password,
			TOTP:       gFlags.totp,
			APIURL:     gFlags.apiURL,
			AppVersion: gFlags.appVersion,
			Output:     parseFormat(gFlags.output),
			LogLevel:   parseLevel(gFlags.logLevel, gFlags.verbose),
			Quiet:      gFlags.quiet,
			DryRun:     gFlags.dryRun,
			FullIDs:    gFlags.fullIDs,
		})
		if err != nil {
			return err
		}
		a.API.SetHVResolver(cliHVResolver(cmd.Context(), a))

		newCtx := app.WithApp(cmd.Context(), a)
		cmd.SetContext(newCtx)
		rootCmd.SetContext(newCtx)
		return nil
	}
}

// Execute runs the root command and exits with the appropriate code.
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	registerSubcommands()

	os.Args = preprocessArgs(os.Args)

	err := rootCmd.ExecuteContext(ctx)
	if err == nil {
		return
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "\nCancelled.")
		os.Exit(130)
	}
	var hvErr *proton.HumanVerificationError
	if errors.As(err, &hvErr) {
		printHVFinalError(os.Stderr, hvErr, app.FromOrNil(rootCmd.Context()))
		os.Exit(exitCode(err))
	}
	err = rewrapFlagError(err, os.Args)
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(exitCode(err))
}

// printHVFinalError formats the user-facing message when a 9001 reaches the
// top level and the captcha helper could not run.
func printHVFinalError(w *os.File, hv *proton.HumanVerificationError, a *app.App) {
	println := func(s string) { _, _ = fmt.Fprintln(w, s) }
	printf := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	println("Error: Proton requires CAPTCHA verification, but proton-cli cannot run a")
	println("webview in this environment.")
	if a != nil && a.HVUnavailableDetail != "" {
		printf("\n  %s\n", a.HVUnavailableDetail)
	}
	println("")
	println("Run the command on a desktop machine with a working webview:")
	println("  Linux:   apt install libwebkit2gtk-4.1-0 libgtk-3-0")
	println("           (or the equivalent for your distro)")
	println("  macOS:   no setup needed (system WebKit)")
	println("  Windows: no setup needed (WebView2 ships with Edge)")
	if len(hv.Methods) > 0 {
		println("")
		printf("Methods Proton offered: %s\n", strings.Join(hv.Methods, ", "))
	}
}

func registerSubcommands() {
	rootCmd.AddCommand(newAPICmd())
	rootCmd.AddCommand(newSettingsCmd())
	rootCmd.AddCommand(newMailCmd())
	rootCmd.AddCommand(newDriveCmd())
	rootCmd.AddCommand(newCalendarCmd())
	rootCmd.AddCommand(newContactsCmd())
	rootCmd.AddCommand(newPassCmd())
}

func parseFormat(s string) render.Format {
	f, err := render.ParseFormat(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	return f
}

func parseLevel(s string, verbose bool) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	if verbose {
		return slog.LevelDebug
	}
	return slog.LevelWarn
}
