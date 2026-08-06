// Command gendocs writes docs/commands/README.md from the command tree itself.
//
// The reference half of the documentation is generated because prose drifts and a
// tree does not. CI regenerates it and fails on a diff, so a command that exists is
// a command that is documented, and one that was renamed cannot keep its old name in
// the docs.
//
// Prose here is written as whole paragraphs on one line, as the hand-written pages
// are: a hard wrap is a decision about somebody else's window.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/roman-16/proton-cli/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("Every command in the tree, generated from the tree itself by `just docs`." +
		" The pages beside this one explain each app; this is the index.\n\n")
	b.WriteString("Anywhere a command shows `REF`, you can pass a full ID, the eight-character short" +
		" ID a list printed, or something human: a subject, a name, a path, an email" +
		" address. See [References](../references.md).\n")

	root := cli.Root()
	for _, group := range root.Groups() {
		fmt.Fprintf(&b, "\n## %s\n", strings.TrimSuffix(group.Title, ":"))
		apps := root.Commands()
		sort.Slice(apps, func(i, j int) bool { return apps[i].Name() < apps[j].Name() })
		for _, app := range apps {
			if app.GroupID != group.ID || app.Hidden {
				continue
			}
			fmt.Fprintf(&b, "\n### `%s`\n\n%s\n\n", app.Name(), app.Short)
			b.WriteString("| Command | What it does |\n| --- | --- |\n")
			writeLeaves(&b, app)
		}
	}

	if err := os.WriteFile("docs/commands/README.md", []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// writeLeaves emits one row per command that does work, in tree order.
func writeLeaves(b *strings.Builder, c *cobra.Command) {
	if c.Runnable() {
		fmt.Fprintf(b, "| `%s` | %s |\n", usage(c), c.Short)
	}
	subs := c.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	for _, sub := range subs {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		writeLeaves(b, sub)
	}
}

// usage is the full invocation, with the binary name dropped so the table stays
// readable.
func usage(c *cobra.Command) string {
	path := strings.TrimPrefix(c.CommandPath(), "proton-cli ")
	if args := strings.Fields(c.Use); len(args) > 1 {
		path += " " + strings.Join(args[1:], " ")
	}
	return path
}
