package cli

import "github.com/spf13/cobra"

// completionCmd exposes cobra's generator under the tool's own group.
//
// It stays visible: completion is one of the two things that make a tree this
// large usable at all, the other being that every leaf documents itself.
func completionCmd(root *cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   "completion",
		Short: "Generate a shell completion script",
		Long: `Generate a shell completion script.

Completion knows the whole command tree, every flag, and the values each
enumerated flag accepts - so it offers folder names, item types, output formats
and setting keys as you type them.

  bash        proton-cli completion bash > /etc/bash_completion.d/proton-cli
  zsh         proton-cli completion zsh > "${fpath[1]}/_proton-cli"
  fish        proton-cli completion fish > ~/.config/fish/completions/proton-cli.fish
  powershell  proton-cli completion powershell | Out-String | Invoke-Expression`,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			default:
				return root.GenPowerShellCompletionWithDesc(out)
			}
		},
	}
	return c
}
