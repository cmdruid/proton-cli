package main

import (
	// Proton stores schedules against IANA zone names (the mail auto-reply,
	// calendar events), so the CLI must resolve them on hosts with no system
	// zone database - Windows and minimal containers among them.
	_ "time/tzdata"

	"github.com/roman-16/proton-cli/internal/cli"
)

func main() {
	cli.Execute()
}
