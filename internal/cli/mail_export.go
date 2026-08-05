package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/roman-16/proton-cli/internal/account/keys"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/spf13/cobra"
)

// Export writes messages back out as RFC 822. Selection reuses the same REFs and
// bulk filters as trash and move, and the output model is the one attachment
// downloads already use, so "export everything from this sender older than a
// year" reads like every other bulk command.

// exportFormat is how selected messages are laid down on disk.
const (
	formatEML  = "eml"
	formatMbox = "mbox"
)

func msgExportCmd() *cobra.Command {
	var f msgFilter
	var format, output, outputDir string
	var force, noAttachments bool
	c := &cobra.Command{
		Use:   "export [REF...]",
		Short: "Write messages out as .eml or mbox files",
		Long: "Write messages out as standalone RFC 822 documents you can open in any mail\n" +
			"client, grep, or feed to other tools.\n\n" +
			"Takes the same filters as trash and move, so a whole folder can be archived\n" +
			"in one command. --format eml writes one file per message into --output-dir;\n" +
			"--format mbox concatenates everything into a single file or stdout.\n\n" +
			"Exported files are NOT encrypted - that is what exporting means. The\n" +
			"original DKIM signatures will not verify against the rebuilt body either,\n" +
			"exactly as with the web client's export.",
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if format != formatEML && format != formatMbox {
				return fmt.Errorf("unknown --format %q (use eml, mbox)", format)
			}
			if output == "-" && format == formatEML && !f.set() && len(c.Args) != 1 {
				return fmt.Errorf("--output - writes one document; select a single message or use --format mbox")
			}
			ids, err := collectMessageIDs(c, c.Args, &f)
			if err != nil {
				return handleWrongTable(err, "export")
			}
			if format == formatEML && len(ids) > 1 && output != "" && output != "-" {
				return fmt.Errorf("--output names one file but %d messages matched; use --output-dir or --format mbox", len(ids))
			}
			if format == formatMbox && outputDir != "" {
				return fmt.Errorf("--format mbox writes one stream; name it with --output")
			}
			if c.dryRun("export %d message(s) as %s", len(ids), format) {
				for _, id := range ids {
					_, _ = fmt.Fprintln(c.R().Stderr, "  "+id)
				}
				return nil
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			return exportMessages(c, u, ids, exportTarget{
				format: format, output: output, outputDir: outputDir,
				force: force, withAttachments: !noAttachments,
			})
		}),
	}
	c.Flags().StringVar(&format, "format", formatEML, "Output format: eml (one file per message), mbox (one stream)")
	c.Flags().StringVar(&output, "output", "", "Write to this path, or - for stdout")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Directory to write per-message files into (default: current directory)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	c.Flags().BoolVar(&noAttachments, "no-attachments", false, "Skip attachments (much faster for a large archive)")
	f.register(c)
	return c
}

type exportTarget struct {
	format          string
	output          string
	outputDir       string
	force           bool
	withAttachments bool
}

func exportMessages(c *Invocation, u *keys.Unlocked, ids []string, t exportTarget) error {
	if t.format == formatMbox {
		return exportMbox(c, u, ids, t)
	}
	return exportEML(c, u, ids, t)
}

// exportEML writes one document per message, naming each after its date and
// subject, and reusing the destination model downloads already follow.
func exportEML(c *Invocation, u *keys.Unlocked, ids []string, t exportTarget) error {
	if t.outputDir != "" {
		if err := ensureDir(t.outputDir); err != nil {
			return err
		}
	}
	for _, id := range ids {
		doc, meta, err := c.App.Mail.Export(c.Ctx, u, id, t.withAttachments)
		if err != nil {
			return err
		}
		name := exportFilename(meta)
		switch {
		case t.output == "-":
			if _, err := c.R().Stdout.Write(doc); err != nil {
				return err
			}
		case t.output != "":
			if err := writeExport(c, doc, t.output, writeError, t.force); err != nil {
				return err
			}
		default:
			if err := writeExport(c, doc, filepath.Join(t.outputDir, name), writeAutoSuffix, t.force); err != nil {
				return err
			}
		}
	}
	c.R().Success(fmt.Sprintf("Exported %d message(s).", len(ids)))
	return nil
}

// exportMbox concatenates every message into one mbox stream, which goes to
// --output or to stdout.
func exportMbox(c *Invocation, u *keys.Unlocked, ids []string, t exportTarget) error {
	out := c.R().Stdout
	if t.output != "" && t.output != "-" {
		if !t.force && fileExists(t.output) {
			return fmt.Errorf("destination %s exists; use --force to overwrite", t.output)
		}
		f, err := os.OpenFile(t.output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	for _, id := range ids {
		doc, meta, err := c.App.Mail.Export(c.Ctx, u, id, t.withAttachments)
		if err != nil {
			return err
		}
		if _, err := out.Write(mailsvc.MboxEntry(doc, meta)); err != nil {
			return err
		}
	}
	c.R().Success(fmt.Sprintf("Exported %d message(s).", len(ids)))
	return nil
}

func writeExport(c *Invocation, doc []byte, path string, mode writeMode, force bool) error {
	if force {
		mode = writeForce
	}
	written, err := writeFileSafe(path, doc, 0o600, mode)
	if err != nil {
		return err
	}
	c.R().Success(fmt.Sprintf("Exported %s (%d bytes)", written, len(doc)))
	return nil
}

// exportFilename names an exported message after when it arrived and what it
// says, which sorts chronologically and stays readable.
func exportFilename(meta *mailsvc.ExportMeta) string {
	stamp := time.Unix(meta.Time, 0).Local().Format("2006-01-02 1504")
	return filepath.Clean(stamp + " " + sanitizeFilename(meta.Subject) + ".eml")
}

// ── conversations export ──

func convExportCmd() *cobra.Command {
	var output string
	var force, noAttachments bool
	c := &cobra.Command{
		Use:   "export REF",
		Short: "Write a whole thread out as an mbox file",
		Long: "Write every message in a thread out as one mbox file, oldest first.\n\n" +
			"Exported files are not encrypted.",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			convID, err := c.App.Mail.ResolveConversation(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			ids, err := c.App.Mail.ConversationMessageIDs(c.Ctx, convID)
			if err != nil {
				return handleWrongTable(err, "export")
			}
			if c.dryRun("export %d message(s) from thread %s", len(ids), convID) {
				return nil
			}
			u, err := c.App.Unlock(c.Ctx)
			if err != nil {
				return err
			}
			return exportMbox(c, u, ids, exportTarget{
				format: formatMbox, output: output,
				force: force, withAttachments: !noAttachments,
			})
		}),
	}
	c.Flags().StringVar(&output, "output", "", "Write to this path (default: stdout)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite an existing file")
	c.Flags().BoolVar(&noAttachments, "no-attachments", false, "Skip attachments")
	return c
}
