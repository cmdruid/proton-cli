package cli

import (
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/render"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/spf13/cobra"
)

// ── mail settings autoreply ──

func mailAutoreplyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "autoreply",
		Short: "Show the auto-reply and its schedule",
		Args:  cobra.NoArgs,
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			ar, err := c.App.Mail.AutoReplyGet(c.Ctx)
			if err != nil {
				return err
			}
			if c.R().Format != render.FormatText {
				return c.R().Object(ar)
			}
			p := fieldPrinter(c, 10)
			p("Status", onOffText(boolInt(ar.Enabled)))
			p("Schedule", ar.ScheduleSummary())
			if ar.Zone != "" && ar.Repeat != "permanent" {
				p("Zone", ar.Zone)
			}
			if ar.Message != "" {
				_, _ = fmt.Fprintf(c.R().Stdout, "\n%s\n", render.HTMLToText(ar.Message))
			}
			return nil
		}),
	}
	c.AddCommand(autoreplySetCmd(),
		autoreplyToggleCmd("enable", "Turn the auto-reply on, keeping its schedule", true),
		autoreplyToggleCmd("disable", "Turn the auto-reply off, keeping its schedule", false))
	return c
}

func autoreplySetCmd() *cobra.Command {
	var ar mailsvc.AutoReply
	var html bool
	c := &cobra.Command{
		Use:   "set",
		Short: "Configure the auto-reply and turn it on",
		Long: "Configure the auto-reply and turn it on.\n\n" +
			"--start and --end are written in the grammar the repeat mode dictates:\n" +
			"  fixed      2026-07-01T09:00   a date and time in --zone\n" +
			"  daily      09:00              a time of day, with --days\n" +
			"  weekly     mon:09:00          a weekday and time\n" +
			"  monthly    1:09:00            a day of the month and time\n" +
			"  permanent  -                  no bounds\n\n" +
			"Proton sends every auto-reply with the subject \"Auto\" and offers no way\n" +
			"to change it, so neither does proton-cli. Auto-reply is a paid feature.",
		Args: cobra.NoArgs,
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			msg, err := readTextArg(ar.Message, "--message")
			if err != nil {
				return err
			}
			if strings.TrimSpace(msg) == "" {
				return fmt.Errorf("--message is required (use - for stdin)")
			}
			if !html {
				msg = render.TextToHTML(msg)
			}
			ar.Message = msg
			if c.dryRun("set the auto-reply (%s)", ar.Repeat) {
				return nil
			}
			if err := c.App.Mail.AutoReplySet(c.Ctx, ar); err != nil {
				return err
			}
			c.R().Success("Auto-reply enabled.")
			return nil
		}),
	}
	c.Flags().StringVar(&ar.Repeat, "repeat", "fixed",
		"Schedule mode: "+strings.Join(mailsvc.RepeatNames(), ", "))
	c.Flags().StringVar(&ar.Start, "start", "", "Start of the window (grammar depends on --repeat)")
	c.Flags().StringVar(&ar.End, "end", "", "End of the window (grammar depends on --repeat)")
	c.Flags().StringSliceVar(&ar.Days, "days", nil, "Days the auto-reply is active (--repeat daily only), e.g. mon,tue,wed")
	c.Flags().StringVar(&ar.Zone, "zone", "", "IANA time zone the schedule is read in (default: the system zone)")
	c.Flags().StringVar(&ar.Message, "message", "", "Reply body (use - for stdin)")
	c.Flags().BoolVar(&html, "html", false, "Treat --message as HTML instead of escaping it and converting newlines")
	return c
}

func autoreplyToggleCmd(use, short string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if c.dryRun("%s the auto-reply", use) {
				return nil
			}
			if err := c.App.Mail.AutoReplyToggle(c.Ctx, enabled); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Auto-reply %sd.", use))
			return nil
		}),
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
