package kit

import "github.com/cmdruid/proton-cli/internal/ui"

// Quantity re-exports the number-agreeing formatter so a command package needs
// one import for the whole vocabulary.
func Quantity(n int, plural string) string { return ui.Quantity(n, plural) }
