// Package cmd wires the Cobra CLI. Command implementations live in the
// per-service subpackages (cmd/mail, cmd/drive, cmd/calendar, cmd/contacts,
// cmd/pass); this package only owns the root command, global flags, and the
// exit-code plumbing.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/roman-16/proton-cli/cmd/calendar"
	"github.com/roman-16/proton-cli/cmd/contacts"
	"github.com/roman-16/proton-cli/cmd/drive"
	"github.com/roman-16/proton-cli/cmd/mail"
	"github.com/roman-16/proton-cli/cmd/pass"
	"github.com/roman-16/proton-cli/internal/api"
	"github.com/roman-16/proton-cli/internal/app"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var version = "dev"

// globalFlags holds persistent flag values; parsed in PersistentPreRunE.
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
	rootCmd.PersistentFlags().StringVar(&gFlags.profile, "profile", "", "config profile to use (default: default)")
	rootCmd.PersistentFlags().StringVar(&gFlags.user, "user", "", "Proton account email (env: PROTON_USER)")
	rootCmd.PersistentFlags().StringVar(&gFlags.password, "password", "", "Account password (env: PROTON_PASSWORD)")
	rootCmd.PersistentFlags().StringVar(&gFlags.totp, "totp", "", "TOTP 2FA code (env: PROTON_TOTP)")
	rootCmd.PersistentFlags().StringVar(&gFlags.apiURL, "api-url", "", "API base URL (env: PROTON_API_URL)")
	rootCmd.PersistentFlags().StringVar(&gFlags.appVersion, "app-version", "", "App version header (env: PROTON_APP_VERSION)")
	rootCmd.PersistentFlags().StringVar(&gFlags.output, "output", "text", "Output format: text, json, yaml")
	rootCmd.PersistentFlags().BoolVar(&gFlags.verbose, "verbose", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&gFlags.quiet, "quiet", false, "Suppress non-essential stderr output")
	rootCmd.PersistentFlags().StringVar(&gFlags.logLevel, "log-level", "", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().BoolVar(&gFlags.dryRun, "dry-run", false, "Preview mutations without applying them")
	rootCmd.PersistentFlags().BoolVar(&gFlags.fullIDs, "full-ids", false, "Show full IDs in interactive output (default: shortened to 8 chars on TTY)")

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
		// Install the HV resolver so the api layer can transparently
		// retry 9001 (human verification) responses by spawning the
		// embedded captcha-helper webview. See cmd/hv.go for the policy.
		a.API.SetHVResolver(cliHVResolver(cmd.Context(), a))

		// Inject the app onto BOTH the active subcommand's context
		// (where service handlers retrieve it via app.From(cmd.Context()))
		// and the root command's context (where Execute()'s final-error
		// formatter retrieves it via app.FromOrNil(rootCmd.Context()) to
		// surface HVUnavailableDetail and similar diagnostics).
		newCtx := app.WithApp(cmd.Context(), a)
		cmd.SetContext(newCtx)
		rootCmd.SetContext(newCtx)
		return nil
	}
}

// Execute runs the root command and exits with the appropriate code.
func Execute() {
	// Cancel context on Ctrl+C / SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	registerSubcommands()

	// Auto-protect Proton IDs that start with '-' so cobra doesn't read them
	// as flags. See preprocessArgs / looksLikeDashedProtonID below.
	os.Args = preprocessArgs(os.Args)

	err := rootCmd.ExecuteContext(ctx)
	if err == nil {
		return
	}
	// If the root context was cancelled (user hit Ctrl+C), show a clean
	// message instead of whatever error chain bubbled up from the layer
	// that noticed the cancellation first (net/http, etc.).
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "\nCancelled.")
		os.Exit(130)
	}
	var hvErr *api.HumanVerificationError
	if errors.As(err, &hvErr) {
		printHVFinalError(os.Stderr, hvErr, app.FromOrNil(rootCmd.Context()))
		os.Exit(app.ExitCodeFor(err))
	}
	err = rewrapFlagError(err, os.Args)
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(app.ExitCodeFor(err))
}

// preprocessArgs walks argv looking for the first token that looks like a
// Proton ID with a leading '-' (length ≥ 60, ends "==", URL-safe base64
// charset). When one is found it injects "--" immediately before it so cobra
// treats the rest as positional. Subsequent leading-dash tokens are protected
// by the same '--' terminator.
//
// The heuristic is strict enough that no real flag value can match: no flag
// in this CLI has a 60+ character base64 value ending in "==".
//
// If the user already has a literal "--" in argv we leave argv alone.
func preprocessArgs(args []string) []string {
	for i := 1; i < len(args); i++ {
		if args[i] == "--" {
			return args
		}
		if looksLikeDashedProtonID(args[i]) {
			out := make([]string, 0, len(args)+1)
			out = append(out, args[:i]...)
			out = append(out, "--")
			out = append(out, args[i:]...)
			return out
		}
	}
	return args
}

// looksLikeDashedProtonID reports whether s starts with a single '-' and
// is otherwise shaped like a Proton ID: at least 60 characters, ends in
// "==", and the body uses only URL-safe base64 characters.
func looksLikeDashedProtonID(s string) bool {
	if len(s) < 60 {
		return false
	}
	if s[0] != '-' || (len(s) > 1 && s[1] == '-') {
		return false
	}
	if !strings.HasSuffix(s, "==") {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '=':
		default:
			return false
		}
	}
	return true
}

// rewrapFlagError translates the two failure modes that surface when a
// leading-dash Proton ID collides with cobra/pflag's flag parsing:
//
//  1. pflag's "unknown shorthand flag: 'X' in -<token>" — surfaces only when
//     preprocessArgs missed the token (heuristic too strict for it).
//  2. cobra's "accepts N arg(s), received M" — surfaces when the user typed
//     flags after the ID; preprocessArgs inserts "--" before the ID, the
//     trailing flags then become positional, and the args validator rejects.
//
// Both rewrites mention `insert -- before it` so users get a consistent hint.
func rewrapFlagError(err error, argv []string) error {
	if err == nil {
		return err
	}

	// Path 1: pflag NotExistError with shorthand token.
	var nee *pflag.NotExistError
	if errors.As(err, &nee) {
		if shorts := nee.GetSpecifiedShortnames(); shorts != "" {
			token := "-" + shorts
			if looksLikeDashedProtonID(token) {
				return fmt.Errorf(
					"that argument looks like a flag because it starts with '-'.\n"+
						"       If it is an ID, insert -- before it:\n"+
						"         proton-cli ... -- %s", token)
			}
		}
	}

	// Path 2: cobra args-validator error + leading-dash ID present in argv.
	msg := err.Error()
	if strings.Contains(msg, "accepts ") && strings.Contains(msg, "arg(s)") {
		for _, a := range argv[1:] {
			if looksLikeDashedProtonID(a) {
				return fmt.Errorf(
					"%w\n"+
						"Hint: %q starts with '-' so it is auto-protected with -- before it.\n"+
						"      Any flags after the ID then become positional arguments. Put flags\n"+
						"      before the ID, or insert -- before it explicitly:\n"+
						"        proton-cli ... --flag value -- %s",
					err, a, a)
			}
		}
	}

	return err
}

// printHVFinalError formats the user-facing message when a 9001
// (human-verification) error reaches the top level. The captcha
// resolver could not run the embedded webview helper here — typically
// because we're in a headless environment (no display server, no GUI
// libraries installed). There is no way to solve the CAPTCHA from
// this process; the user has to run the command on a desktop machine.
//
// If the resolver stashed a specific diagnostic on
// app.HVUnavailableDetail, we surface it so the user knows WHY the
// helper couldn't run (no $DISPLAY, missing libwebkit2gtk, etc.).
func printHVFinalError(w *os.File, hv *api.HumanVerificationError, a *app.App) {
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
	rootCmd.AddCommand(mail.NewCmd())
	rootCmd.AddCommand(drive.NewCmd())
	rootCmd.AddCommand(calendar.NewCmd())
	rootCmd.AddCommand(contacts.NewCmd())
	rootCmd.AddCommand(pass.NewCmd())
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
