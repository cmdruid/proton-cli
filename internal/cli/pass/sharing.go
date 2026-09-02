package pass

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cmdruid/proton-cli/internal/cli/kit"
	passsvc "github.com/cmdruid/proton-cli/internal/service/pass"
	"github.com/cmdruid/proton-cli/internal/ui"
)

// Sharing a vault, and taking one somebody shared with you.
//
// A vault is opened by its share key and every item in it is sealed under that
// key, so sharing means handing over the key itself - every rotation of it,
// because an item made before the last rotation is still sealed under an older
// one. It goes out encrypted to their key and signed with yours.

func vaultsShareCmd() *cobra.Command {
	c := &cobra.Command{Use: "share", Short: "Who else can open a vault"}
	c.AddCommand(vaultShareAddCmd(), vaultShareListCmd(), vaultShareRemoveCmd())
	return c
}

func inviteColumns() []ui.Column[passsvc.VaultInvite] {
	return []ui.Column[passsvc.VaultInvite]{
		{Header: "ID", ID: true, Cell: func(i passsvc.VaultInvite) string { return i.ID }},
		{Header: "EMAIL", Flex: true, Cell: func(i passsvc.VaultInvite) string { return i.Email }},
		{Header: "ACCESS", Cell: func(i passsvc.VaultInvite) string { return i.Access }},
	}
}

func vaultShareAddCmd() *cobra.Command {
	access := &kit.Enum{
		Name: "access", Usage: "What they may do with it",
		Values: passsvc.VaultRoles(), Default: "viewer",
	}
	c := &cobra.Command{
		Use:   "add REF EMAIL",
		Short: "Offer a vault to somebody",
		Long: "Offer a vault to somebody.\n\n" +
			"They are sent an invitation and see nothing until they take it. What is\n" +
			"sent is the key that opens the vault, encrypted to their key and signed\n" +
			"with yours - so it has to be another Proton account, because an address\n" +
			"Proton holds no keys for has nothing to encrypt to.",
		Args: cobra.ExactArgs(2),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			role, err := access.Value()
			if err != nil {
				return err
			}
			vault, err := vaultList(c).Find(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Invited, Kind: "invitations", Count: 1, Name: c.Args[1],
				Detail: "to " + vault.Name,
			}, func() error {
				return c.App.Pass.VaultShare(c.Ctx, vault.ShareID, c.Args[1], role)
			})
		}),
	}
	access.Register(c)
	return c
}

func vaultShareListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list REF",
		Short: "List who has been offered a vault",
		Args:  cobra.ExactArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			vault, err := vaultList(c).Find(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			rows, err := c.App.Pass.VaultInvitesSent(c.Ctx, vault.ShareID)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[passsvc.VaultInvite]{
				Noun: "invitations", Columns: inviteColumns(), Total: len(rows), Page: ui.Unpaged,
			}, rows, func(i passsvc.VaultInvite) []string { return []string{i.ID} })
		}),
	}
}

func vaultShareRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove REF EMAIL",
		Short: "Withdraw an offer nobody has taken",
		Args:  cobra.ExactArgs(2),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			vault, err := vaultList(c).Find(c.Ctx, c.Args[0])
			if err != nil {
				return err
			}
			invite, err := sentInviteList(c, vault.ShareID).Find(c.Ctx, c.Args[1])
			if err != nil {
				return err
			}
			return kit.Mutate(c, ui.ResultSpec{
				Action: ui.Revoked, Kind: "invitations", Count: 1, Name: invite.Email,
				Detail: "to " + vault.Name, IDs: []string{invite.ID},
			}, func() error {
				return c.App.Pass.VaultInviteRevoke(c.Ctx, vault.ShareID, invite.ID)
			})
		}),
	}
}

func sentInviteList(c *kit.Invocation, shareID string) *kit.Lookup[passsvc.VaultInvite] {
	return &kit.Lookup[passsvc.VaultInvite]{
		Kind: "invitation",
		Load: func(ctx context.Context) ([]passsvc.VaultInvite, error) {
			return c.App.Pass.VaultInvitesSent(ctx, shareID)
		},
		ID:     func(i passsvc.VaultInvite) string { return i.ID },
		Handle: func(i passsvc.VaultInvite) string { return i.Email },
	}
}

// ── the other side ──

func invitationsCmd() *cobra.Command {
	c := &cobra.Command{Use: "invitations", Short: "Vaults other people have offered you"}
	c.AddCommand(invitationsListCmd(), invitationsAcceptCmd(), invitationsDeclineCmd())
	return c
}

func receivedList(c *kit.Invocation) *kit.Lookup[passsvc.VaultInvite] {
	return &kit.Lookup[passsvc.VaultInvite]{
		Kind: "invitation",
		Load: func(ctx context.Context) ([]passsvc.VaultInvite, error) {
			return c.App.Pass.VaultInvitesReceived(ctx)
		},
		ID:     func(i passsvc.VaultInvite) string { return i.ID },
		Handle: func(i passsvc.VaultInvite) string { return i.Vault },
	}
}

func receivedColumns() []ui.Column[passsvc.VaultInvite] {
	return []ui.Column[passsvc.VaultInvite]{
		{Header: "ID", ID: true, Cell: func(i passsvc.VaultInvite) string { return i.ID }},
		{Header: "VAULT", Flex: true, Cell: func(i passsvc.VaultInvite) string { return i.Vault }},
		{Header: "FROM", Cell: func(i passsvc.VaultInvite) string { return i.Inviter }},
		{Header: "ACCESS", Cell: func(i passsvc.VaultInvite) string { return i.Access }},
		{Header: "ITEMS", Right: true, Cell: func(i passsvc.VaultInvite) string {
			return strconv.Itoa(i.Items)
		}},
	}
}

func invitationsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List vaults other people have offered you",
		Long: "List vaults other people have offered you.\n\n" +
			"The vault's name and how much is in it are readable before you take it:\n" +
			"the invitation carries the key that opens them, encrypted to you. What is\n" +
			"in the vault is not, until you accept.",
		Args: cobra.NoArgs,
		RunE: kit.Run(nil, func(c *kit.Invocation) error {
			rows, err := c.App.Pass.VaultInvitesReceived(c.Ctx)
			if err != nil {
				return err
			}
			return kit.List(c, ui.TableSpec[passsvc.VaultInvite]{
				Noun: "invitations", Columns: receivedColumns(), Total: len(rows), Page: ui.Unpaged,
			}, rows, func(i passsvc.VaultInvite) []string { return []string{i.ID} })
		}),
	}
}

func invitationsAcceptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "accept REF...",
		Short: "Take a vault somebody offered you",
		Long: "Take a vault somebody offered you.\n\n" +
			"The keys arrive encrypted to the address the offer was sent to and are\n" +
			"moved onto your own key, which is what makes the vault open like any\n" +
			"other of yours afterwards.",
		Args: cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			return answerVaultInvites(c, ui.Accepted, c.App.Pass.VaultInviteAccept)
		}),
	}
}

func invitationsDeclineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decline REF...",
		Short: "Turn down a vault somebody offered you",
		Args:  cobra.MinimumNArgs(1),
		RunE: kit.Run([]kit.Step{kit.StepExpand}, func(c *kit.Invocation) error {
			return answerVaultInvites(c, ui.Declined, c.App.Pass.VaultInviteReject)
		}),
	}
}

// answerVaultInvites says yes or no to the ones named.
func answerVaultInvites(c *kit.Invocation, action ui.Action, answer func(context.Context, string) error) error {
	sel, err := kit.SelectFrom(c, "invitations", receivedColumns(), receivedList(c))
	if err != nil {
		return err
	}
	return kit.Mutate(c, ui.ResultSpec{
		Action: action, Kind: "invitations", Count: sel.Len(), IDs: sel.IDs,
		Name: kit.Sole(sel.Rows, func(i passsvc.VaultInvite) string { return i.Vault }),
	}, func() error {
		for _, id := range sel.IDs {
			if err := answer(c.Ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}
