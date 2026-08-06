// Package cli wires the Cobra command tree. Command bodies are prepared by the
// pipeline (auth/unlock/resolve steps); this file owns the root command, global
// flags, exit-code plumbing and the leading-dash-ID workaround.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/roman-16/proton-cli/internal/app"
	accountcmd "github.com/roman-16/proton-cli/internal/cli/account"
	apicmd "github.com/roman-16/proton-cli/internal/cli/api"
	calendarcmd "github.com/roman-16/proton-cli/internal/cli/calendar"
	contactscmd "github.com/roman-16/proton-cli/internal/cli/contacts"
	drivecmd "github.com/roman-16/proton-cli/internal/cli/drive"
	mailcmd "github.com/roman-16/proton-cli/internal/cli/mail"
	passcmd "github.com/roman-16/proton-cli/internal/cli/pass"
	selfcmd "github.com/roman-16/proton-cli/internal/cli/self"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ui"
	"github.com/spf13/cobra"
)

// version is overridden at release time via -ldflags -X.
var version = "dev"

type globalFlags struct {
	profile    string
	user       string
	password   string
	totp       string
	apiURL     string
	appVersion string
	output     string
	quiet      bool
	logLevel   string
	dryRun     bool
	fullIDs    bool
	noColor    bool
	noInput    bool
}

// Command groups on the root, so `--help` reads as a map of the product rather
// than an alphabetical list.
const (
	groupApps    = "apps"
	groupAccount = "account"
	groupSelf    = "self"
)

// newRoot assembles the whole command tree and returns it.
//
// Building the tree rather than mutating a package variable is what lets the
// conformance test walk a complete, freshly constructed tree and check the rules
// the interface is meant to obey.
func newRoot() *cobra.Command {
	var g globalFlags

	root := &cobra.Command{
		Use:   "proton-cli",
		Short: "Unofficial CLI for Proton Mail, Drive, Calendar, Pass and Contacts",
		Long: "Proton, in your terminal.\n\n" +
			"Unofficial, end-to-end encrypted CLI for Proton Mail, Drive, Calendar, Pass and Contacts.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&g.profile, "profile", "", "Profile to use (env: PROTON_PROFILE; default: default)")
	pf.StringVar(&g.user, "user", "", "Proton account email (env: PROTON_USER, or PROTON_<PROFILE>_USER)")
	pf.StringVar(&g.password, "password", "", "Account password (env: PROTON_PASSWORD, or PROTON_<PROFILE>_PASSWORD)")
	pf.StringVar(&g.totp, "totp", "", "TOTP 2FA code (env: PROTON_TOTP, or PROTON_<PROFILE>_TOTP)")
	pf.StringVar(&g.apiURL, "api-url", "", "API base URL (env: PROTON_API_URL, or PROTON_<PROFILE>_API_URL)")
	pf.StringVar(&g.appVersion, "app-version", "", "App version header (env: PROTON_APP_VERSION, or PROTON_<PROFILE>_APP_VERSION)")
	pf.StringVar(&g.output, "output", "text", "Output format: text, json, yaml")
	pf.BoolVar(&g.quiet, "quiet", false, "Suppress non-essential stderr output")
	pf.StringVar(&g.logLevel, "log-level", "",
		"Logging verbosity: "+strings.Join(ui.LogLevels, ", ")+" (env: PROTON_LOG_LEVEL)")
	pf.BoolVar(&g.dryRun, "dry-run", false, "Preview mutations without applying them")
	pf.BoolVar(&g.fullIDs, "full-ids", false, "Show full IDs in interactive output (default: shortened to 8 chars on TTY)")
	pf.BoolVar(&g.noColor, "no-color", false, "Disable colored output (env: NO_COLOR)")
	pf.BoolVar(&g.noInput, "no-input", false, "Never prompt; a missing credential becomes an error (env: PROTON_NO_INPUT)")

	// Parse each level's flags while walking to the target command, so an
	// unrecognised flag is reported as one.
	//
	// Without this, cobra fails to route and blames the subcommand instead:
	// `proton-cli --bogus account get` answers `Unknown command "get"`, which
	// points a reader at the wrong thing entirely.
	root.TraverseChildren = true

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		format, err := ui.ParseFormat(g.output)
		if err != nil {
			return err
		}
		// Both are judged here, before any command body runs and so before
		// anything reaches the network.
		level, err := ui.ParseLogLevel(g.logLevel)
		if err != nil {
			return err
		}
		a, err := app.New(app.Options{
			Profile:    g.profile,
			User:       g.user,
			Password:   g.password,
			TOTP:       g.totp,
			APIURL:     g.apiURL,
			AppVersion: g.appVersion,
			Version:    version,
			Output:     format,
			LogLevel:   level,
			Quiet:      g.quiet,
			DryRun:     g.dryRun,
			FullIDs:    g.fullIDs,
			NoColor:    g.noColor,
			NoInput:    g.noInput,
		})
		if err != nil {
			return err
		}
		a.API.SetHVResolver(cliHVResolver(cmd.Context(), a))

		newCtx := app.WithApp(cmd.Context(), a)
		cmd.SetContext(newCtx)
		root.SetContext(newCtx)
		return nil
	}

	root.AddGroup(
		&cobra.Group{ID: groupApps, Title: "Apps:"},
		&cobra.Group{ID: groupAccount, Title: "Account:"},
		&cobra.Group{ID: groupSelf, Title: "proton-cli itself:"},
	)

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add(groupApps, mailcmd.New(), drivecmd.New(), calendarcmd.New(), contactscmd.New(), passcmd.New())
	add(groupAccount, accountcmd.New(), apicmd.New())
	add(groupSelf, selfcmd.UpdateCmd(version), selfcmd.UninstallCmd(),
		selfcmd.VersionCmd(version), completionCmd(root))

	return root
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := newRoot()
	os.Args = preprocessArgs(os.Args)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "\nCancelled.")
		os.Exit(130)
	}
	var hvErr *proton.HumanVerificationError
	if errors.As(err, &hvErr) {
		printHVFinalError(os.Stderr, hvErr, app.FromOrNil(root.Context()))
		os.Exit(exitCode(err))
	}
	ui.WriteError(os.Stderr, rewrapFlagError(err, os.Args), errorTheme(root))
	os.Exit(exitCode(err))
}

// errorTheme is the palette for the final error. The app owns one, but a failure
// during flag parsing happens before there is an app, so fall back to asking the
// stream directly.
func errorTheme(root *cobra.Command) ui.Theme {
	if a := app.FromOrNil(root.Context()); a != nil {
		return a.UI.ErrTheme()
	}
	return ui.ThemeFor(os.Stderr)
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

// Root returns the assembled command tree, for the documentation generator and
// for anything else that needs to walk it.
func Root() *cobra.Command { return newRoot() }
