package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/errs"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/roman-16/proton-cli/internal/view"
	"github.com/spf13/cobra"
)

// ── mail attachments ──

func attachmentsCmd() *cobra.Command {
	c := &cobra.Command{Use: "attachments", Short: "Manage message attachments"}
	c.AddCommand(attachmentsListCmd(), attachmentDownloadCmd())
	return c
}

func attachmentsListCmd() *cobra.Command {
	var includeInline bool
	c := &cobra.Command{
		Use: "list MESSAGE_ID", Short: "List attachments of a message",
		Args: cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			atts, err := c.App.Mail.AttachmentsList(c.Ctx, c.Args[0], includeInline)
			if err != nil {
				return err
			}
			cols := []view.Column[mailsvc.Attachment]{
				{Header: "ID", ID: true, Cell: func(a mailsvc.Attachment) string { return a.ID }},
				{Header: "NAME", Cell: func(a mailsvc.Attachment) string { return a.Name }},
				{Header: "SIZE", Cell: func(a mailsvc.Attachment) string { return units.Size(a.Size) }},
				{Header: "TYPE", Cell: func(a mailsvc.Attachment) string { return a.MIMEType }},
			}
			if includeInline {
				cols = append(cols, view.Column[mailsvc.Attachment]{Header: "DISPOSITION", Cell: func(a mailsvc.Attachment) string {
					if a.Disposition == "" {
						return "attachment"
					}
					return a.Disposition
				}})
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.Attachment]{
				Columns:  cols,
				CacheIDs: func(a mailsvc.Attachment) []string { return []string{a.ID} },
			}, atts)
		}),
	}
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) hidden by default")
	return c
}

func attachmentDownloadCmd() *cobra.Command {
	var output, outputDir string
	var all, force, includeInline bool
	c := &cobra.Command{
		Use:   "download MESSAGE_ID [ATTACHMENT_ID]",
		Short: "Download and decrypt attachment(s) (--output - for stdout, --all for every attachment)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			msgID := args[0]
			var attID string
			if len(args) == 2 {
				attID = args[1]
			}
			if err := validateDownloadShape(attID, output, outputDir, all); err != nil {
				return err
			}
			return run([]Step{stepAuth}, func(c *Invocation) error {
				u, err := c.App.Unlock(c.Ctx)
				if err != nil {
					return err
				}
				if msgID, err = resolvePrefix(c.App, msgID); err != nil {
					return err
				}
				if attID, err = resolvePrefix(c.App, attID); err != nil {
					return err
				}
				if all {
					return downloadAllAttachments(c, u, msgID, outputDir, force, includeInline)
				}
				return downloadOneAttachment(c, u, msgID, attID, output, outputDir, force)
			})(cmd, args)
		},
	}
	c.Flags().StringVar(&output, "output", "", "Explicit output path (- for stdout); errors on existing file")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the attachment's own name (auto-suffix on collision)")
	c.Flags().BoolVar(&all, "all", false, "Download every attachment on the message (requires --output-dir)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing destination files")
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) when --all")
	return c
}

func validateDownloadShape(idArg, output, outputDir string, all bool) error {
	if output != "" && outputDir != "" {
		return fmt.Errorf("specify either --output or --output-dir, not both")
	}
	if all {
		if idArg != "" {
			return fmt.Errorf("--all does not take an ATTACHMENT_ID")
		}
		if output != "" {
			if output == "-" {
				return fmt.Errorf("--all cannot write to stdout")
			}
			return fmt.Errorf("--all requires --output-dir, not --output")
		}
		if outputDir == "" {
			return fmt.Errorf("--all requires --output-dir")
		}
	} else if idArg == "" {
		return fmt.Errorf("ATTACHMENT_ID is required (or use --all)")
	}
	return nil
}

// dispatchAttachmentBytes writes decrypted attachment bytes to the destination
// implied by the (already-validated) --output / --output-dir flags, using the
// shared download model: --output - is stdout, --output PATH errors on an
// existing file, --output-dir DIR (and the flagless default) write the
// attachment's own name into DIR / the current directory, auto-suffixing on
// collision. name is the attachment's own filename.
func dispatchAttachmentBytes(c *Invocation, bin []byte, name, output, outputDir string, force bool) error {
	switch {
	case output == "-":
		_, err := c.R().Stdout.Write(bin)
		return err
	case output != "":
		return writeAttachment(c, bin, output, writeError, force)
	case outputDir != "":
		if err := ensureDir(outputDir); err != nil {
			return err
		}
		return writeAttachment(c, bin, filepath.Join(outputDir, name), writeAutoSuffix, force)
	default:
		return writeAttachment(c, bin, name, writeAutoSuffix, force)
	}
}

func downloadOneAttachment(c *Invocation, u *keys.Unlocked, msgID, attID, output, outputDir string, force bool) error {
	bin, name, err := c.App.Mail.AttachmentDownload(c.Ctx, u, msgID, attID)
	if err != nil {
		return err
	}
	return dispatchAttachmentBytes(c, bin, name, output, outputDir, force)
}

func downloadAllAttachments(c *Invocation, u *keys.Unlocked, msgID, outputDir string, force, includeInline bool) error {
	atts, err := c.App.Mail.AttachmentsList(c.Ctx, msgID, includeInline)
	if err != nil {
		return err
	}
	if len(atts) == 0 {
		c.R().Info("no attachments to download")
		return nil
	}
	if err := ensureDir(outputDir); err != nil {
		return err
	}
	for _, at := range atts {
		bin, _, err := c.App.Mail.AttachmentDownload(c.Ctx, u, msgID, at.ID)
		if err != nil {
			return fmt.Errorf("download %s (%s): %w", at.Name, at.ID, err)
		}
		if err := writeAttachment(c, bin, filepath.Join(outputDir, at.Name), writeAutoSuffix, force); err != nil {
			return err
		}
	}
	c.R().Success(fmt.Sprintf("Downloaded %d attachment(s) to %s", len(atts), outputDir))
	return nil
}

func writeAttachment(c *Invocation, data []byte, path string, mode writeMode, force bool) error {
	if force {
		mode = writeForce
	}
	written, err := writeFileSafe(path, data, 0644, mode)
	if err != nil {
		return err
	}
	c.R().Success(fmt.Sprintf("Downloaded %s (%d bytes)", written, len(data)))
	return nil
}

func ensureDir(dir string) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", dir)
		}
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// ── mail conversations attachments ──

func convAttachmentsCmd() *cobra.Command {
	c := &cobra.Command{Use: "attachments", Short: "Manage attachments across a conversation"}
	c.AddCommand(convAttachmentsListCmd(), convAttachmentDownloadCmd())
	return c
}

func convAttachmentsListCmd() *cobra.Command {
	var includeInline bool
	c := &cobra.Command{
		Use:   "list CONVERSATION_ID",
		Short: "List attachments across all messages in a conversation",
		Args:  cobra.ExactArgs(1),
		RunE: run([]Step{stepAuth, stepResolve}, func(c *Invocation) error {
			atts, err := c.App.Mail.ConversationAttachmentsList(c.Ctx, c.Args[0], includeInline)
			if err != nil {
				return handleWrongTable(err, "attachments list")
			}
			cols := []view.Column[mailsvc.ConversationAttachment]{
				{Header: "ID", ID: true, Cell: func(a mailsvc.ConversationAttachment) string { return a.ID }},
				{Header: "NAME", Cell: func(a mailsvc.ConversationAttachment) string { return a.Name }},
				{Header: "SIZE", Cell: func(a mailsvc.ConversationAttachment) string { return units.Size(a.Size) }},
				{Header: "TYPE", Cell: func(a mailsvc.ConversationAttachment) string { return a.MIMEType }},
				{Header: "MESSAGE_ID", ID: true, Cell: func(a mailsvc.ConversationAttachment) string { return a.MessageID }},
			}
			if includeInline {
				cols = append(cols, view.Column[mailsvc.ConversationAttachment]{Header: "DISPOSITION", Cell: func(a mailsvc.ConversationAttachment) string {
					if a.Disposition == "" {
						return "attachment"
					}
					return a.Disposition
				}})
			}
			return view.Render(c.R(), c.short(), c.App.IDCache, view.List[mailsvc.ConversationAttachment]{
				Columns:  cols,
				CacheIDs: func(a mailsvc.ConversationAttachment) []string { return []string{a.ID, a.MessageID} },
			}, atts)
		}),
	}
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) hidden by default")
	return c
}

func convAttachmentDownloadCmd() *cobra.Command {
	var output, outputDir string
	var all, force, includeInline bool
	c := &cobra.Command{
		Use:   "download CONVERSATION_ID [ATTACHMENT_ID]",
		Short: "Download and decrypt attachment(s) from a conversation",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := args[0]
			var attID string
			if len(args) == 2 {
				attID = args[1]
			}
			if err := validateDownloadShape(attID, output, outputDir, all); err != nil {
				return err
			}
			return run([]Step{stepAuth}, func(c *Invocation) error {
				u, err := c.App.Unlock(c.Ctx)
				if err != nil {
					return err
				}
				if convID, err = resolvePrefix(c.App, convID); err != nil {
					return err
				}
				if attID, err = resolvePrefix(c.App, attID); err != nil {
					return err
				}
				if all {
					return downloadAllConvAttachments(c, u, convID, outputDir, force, includeInline)
				}
				return downloadOneConvAttachment(c, u, convID, attID, output, outputDir, force)
			})(cmd, args)
		},
	}
	c.Flags().StringVar(&output, "output", "", "Explicit output path (- for stdout); errors on existing file")
	c.Flags().StringVar(&outputDir, "output-dir", "", "Output directory; uses the attachment's own name (auto-suffix on collision)")
	c.Flags().BoolVar(&all, "all", false, "Download every attachment in the conversation (requires --output-dir)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing destination files")
	c.Flags().BoolVar(&includeInline, "include-inline", false, "Include inline attachments (e.g. signature graphics) when --all")
	return c
}

func downloadOneConvAttachment(c *Invocation, u *keys.Unlocked, convID, attID, output, outputDir string, force bool) error {
	list, err := c.App.Mail.ConversationAttachmentsList(c.Ctx, convID, true)
	if err != nil {
		return handleWrongTable(err, "attachments download")
	}
	var msgID, name string
	for _, at := range list {
		if at.ID == attID {
			msgID, name = at.MessageID, at.Name
			break
		}
	}
	if msgID == "" {
		return errs.WithExit(3, fmt.Errorf("attachment %s not found in conversation %s", attID, convID))
	}
	bin, _, err := c.App.Mail.AttachmentDownload(c.Ctx, u, msgID, attID)
	if err != nil {
		return err
	}
	return dispatchAttachmentBytes(c, bin, name, output, outputDir, force)
}

func downloadAllConvAttachments(c *Invocation, u *keys.Unlocked, convID, outputDir string, force, includeInline bool) error {
	list, err := c.App.Mail.ConversationAttachmentsList(c.Ctx, convID, includeInline)
	if err != nil {
		return handleWrongTable(err, "attachments download")
	}
	if len(list) == 0 {
		c.R().Info("no attachments to download")
		return nil
	}
	if err := ensureDir(outputDir); err != nil {
		return err
	}
	for _, at := range list {
		bin, _, err := c.App.Mail.AttachmentDownload(c.Ctx, u, at.MessageID, at.ID)
		if err != nil {
			return fmt.Errorf("download %s (%s): %w", at.Name, at.ID, err)
		}
		if err := writeAttachment(c, bin, filepath.Join(outputDir, at.Name), writeAutoSuffix, force); err != nil {
			return err
		}
	}
	c.R().Success(fmt.Sprintf("Downloaded %d attachment(s) to %s", len(list), outputDir))
	return nil
}

// renderAttachmentsFooter returns the text-mode footer block for a message's
// attachments, or "" when nothing is shown. Shared by message- and
// conversation-read rendering.
func renderAttachmentsFooter(atts []mailsvc.Attachment, includeInline bool) string {
	visible := atts
	if !includeInline {
		visible = mailsvc.FilterInline(atts)
	}
	if len(visible) == 0 {
		return ""
	}
	sizes := make([]string, len(visible))
	var maxName, maxSize int
	for i, a := range visible {
		sizes[i] = units.Size(a.Size)
		if n := utf8.RuneCountInString(a.Name); n > maxName {
			maxName = n
		}
		if n := utf8.RuneCountInString(sizes[i]); n > maxSize {
			maxSize = n
		}
	}
	var b strings.Builder
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "Attachments (%d):\n", len(visible))
	for i, a := range visible {
		sizeCell := "(" + sizes[i] + ")" + strings.Repeat(" ", maxSize-utf8.RuneCountInString(sizes[i]))
		fmt.Fprintf(&b, "  - %s  %s  ID: %s", padName(a.Name, maxName), sizeCell, a.ID)
		if a.IsInline() {
			b.WriteString("  (inline)")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func padName(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
