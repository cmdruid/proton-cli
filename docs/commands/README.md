# Command reference

Every command in the tree, generated from the tree itself by `just docs`. The pages beside this one explain each app; this is the index.

Anywhere a command shows `REF`, you can pass a full ID, the eight-character short ID a list printed, or something human: a subject, a name, a path, an email address. See [References](../references.md).

## Apps

### `calendar`

Calendars and events

| Command | What it does |
| --- | --- |
| `calendar events create` | Create an event |
| `calendar events delete REF...` | Delete events |
| `calendar events export` | Write events out as an .ics file |
| `calendar events get REF` | Show one event, decrypted |
| `calendar events import PATH` | Read events in from an .ics file |
| `calendar events list` | List events in a date range |
| `calendar events respond REF` | Answer an invitation, telling the organizer |
| `calendar events update REF` | Change an event's title, time, location, description or recurrence |
| `calendar invitations accept REF...` | Take a calendar somebody offered you |
| `calendar invitations decline REF...` | Turn down a calendar somebody offered you |
| `calendar invitations list` | List calendars other people have offered you |
| `calendar settings calendars create` | Create a calendar, or subscribe to one published elsewhere |
| `calendar settings calendars delete REF...` | Delete calendars, and every event in them |
| `calendar settings calendars get REF` | Show one calendar, with the defaults it gives new events |
| `calendar settings calendars list` | List your calendars |
| `calendar settings calendars share add REF EMAIL` | Give somebody a calendar |
| `calendar settings calendars share list REF` | List who has a calendar |
| `calendar settings calendars share remove REF EMAIL` | Take somebody's access to a calendar away |
| `calendar settings calendars update REF` | Rename or recolor a calendar, or change what it gives new events |
| `calendar settings get` | Show the calendar settings now in effect |
| `calendar settings list` | List the calendar settings that can be changed |
| `calendar settings set KEY VALUE` | Change one calendar setting |

### `contacts`

Contacts, their groups and their pinned keys

| Command | What it does |
| --- | --- |
| `contacts create` | Create a contact |
| `contacts delete REF...` | Delete contacts |
| `contacts export [REF...]` | Write contacts out as vCards |
| `contacts get REF` | Show one contact in full |
| `contacts groups add REF CONTACT_REF...` | Add contacts to a group |
| `contacts groups create` | Create a contact group |
| `contacts groups delete REF...` | Delete contact groups |
| `contacts groups get REF` | Show one group and the addresses in it |
| `contacts groups list` | List contact groups |
| `contacts groups remove REF CONTACT_REF...` | Remove contacts from a group |
| `contacts groups update REF` | Rename or recolor a contact group |
| `contacts import PATH` | Read contacts in from a .vcf file |
| `contacts keys list REF` | List the keys pinned to a contact |
| `contacts keys pin REF` | Pin a public key so mail to a contact is encrypted to it |
| `contacts keys unpin REF` | Remove the keys pinned to a contact |
| `contacts list` | List contacts |
| `contacts merge` | Fold duplicate contacts into one |
| `contacts update REF` | Change a contact's details |

### `drive`

Files and folders in Drive

| Command | What it does |
| --- | --- |
| `drive invitations accept REF...` | Accept invitations |
| `drive invitations decline REF...` | Decline invitations |
| `drive invitations list` | List invitations waiting for an answer |
| `drive items copy [PATH...]` | Copy files into another folder |
| `drive items create PATH` | Create a folder, and any missing folder above it |
| `drive items delete [PATH...]` | Delete files or folders permanently |
| `drive items download PATH` | Download a file |
| `drive items get PATH` | Show a file or folder's details |
| `drive items list [PATH]` | List what is in a folder |
| `drive items move [PATH...]` | Move files or folders into another folder |
| `drive items revisions delete PATH REVISION_REF` | Delete an earlier version permanently |
| `drive items revisions download PATH REVISION_REF` | Download an earlier version of a file |
| `drive items revisions list PATH` | List a file's earlier versions |
| `drive items revisions restore PATH REVISION_REF` | Restore a file to an earlier version |
| `drive items share add PATH EMAIL` | Invite someone to a file or folder |
| `drive items share get PATH` | Show how a file or folder is shared |
| `drive items share link PATH` | Create or update the public link for a file or folder |
| `drive items share remove PATH EMAIL` | Revoke someone's access, or cancel their invitation |
| `drive items share resend PATH EMAIL` | Send an unanswered invitation again |
| `drive items share unlink PATH` | Remove the public links for a file or folder |
| `drive items share update PATH EMAIL` | Change what somebody may do with a file or folder |
| `drive items trash [PATH...]` | Move files or folders to the trash |
| `drive items update PATH` | Rename a file or folder |
| `drive items upload SRC [DEST]` | Upload a file or directory |
| `drive photos albums add REF PHOTO_REF...` | Put photos into an album |
| `drive photos albums create` | Create an album |
| `drive photos albums delete REF...` | Delete albums |
| `drive photos albums list` | List albums |
| `drive photos albums remove REF PHOTO_REF...` | Take photos out of an album |
| `drive photos albums update REF` | Change an album's cover |
| `drive photos delete REF...` | Delete photos permanently |
| `drive photos download REF` | Download a photo |
| `drive photos favorite REF...` | Mark photos as favourites |
| `drive photos list` | List photos |
| `drive photos trash REF...` | Move photos to the trash |
| `drive photos unfavorite REF...` | Remove photos from favourites |
| `drive photos upload SRC` | Upload a photo to the library |
| `drive settings get` | Show the drive settings now in effect |
| `drive settings list` | List the drive settings that can be changed |
| `drive settings set KEY VALUE` | Change one drive setting |
| `drive shared list` | List what other people have shared with you |
| `drive sharing list` | List what you have shared |
| `drive trash empty` | Delete everything in the trash, permanently |
| `drive trash list` | List what is in the trash |
| `drive trash restore REF...` | Put items back where they came from |

### `mail`

Read, write and organize mail

| Command | What it does |
| --- | --- |
| `mail conversations attachments download REF [ATTACHMENT_REF]` | Download and decrypt attachments from a thread |
| `mail conversations attachments list REF` | List every attachment in a thread |
| `mail conversations delete [REF...]` | Delete threads permanently |
| `mail conversations export REF` | Write a whole thread out as .eml files or one mbox |
| `mail conversations forward REF` | Forward the newest message in a thread |
| `mail conversations get REF` | Show a whole thread, decrypted |
| `mail conversations label [REF...]` | Attach a label to threads |
| `mail conversations list` | List threads in a folder |
| `mail conversations mark read [REF...]` | Mark threads as read |
| `mail conversations mark unread [REF...]` | Mark threads as unread |
| `mail conversations move [REF...]` | Move threads to a folder |
| `mail conversations reply REF` | Reply to the newest message in a thread |
| `mail conversations snooze [REF...]` | Take threads out of the inbox until later |
| `mail conversations star [REF...]` | Star threads |
| `mail conversations trash [REF...]` | Move threads to the trash |
| `mail conversations unlabel [REF...]` | Detach a label from threads |
| `mail conversations unsnooze [REF...]` | Bring snoozed threads back to the inbox now |
| `mail conversations unstar [REF...]` | Remove the star from threads |
| `mail drafts create` | Save a draft without sending it |
| `mail drafts delete REF...` | Delete drafts |
| `mail drafts list` | List drafts |
| `mail drafts send REF` | Send a draft as it stands |
| `mail drafts update REF` | Change a draft's recipients, subject, body or attachments |
| `mail messages attachments download REF [ATTACHMENT_REF]` | Download and decrypt attachments |
| `mail messages attachments list REF` | List a message's attachments |
| `mail messages delete [REF...]` | Delete messages permanently |
| `mail messages empty` | Delete everything in a folder, permanently |
| `mail messages expire [REF...]` | Make messages delete themselves after a while |
| `mail messages export [REF...]` | Write messages out as .eml or mbox files |
| `mail messages forward REF` | Forward a message |
| `mail messages get REF` | Show one message, decrypted |
| `mail messages label [REF...]` | Attach a label to messages |
| `mail messages list` | List messages in a folder |
| `mail messages mark read [REF...]` | Mark messages as read |
| `mail messages mark unread [REF...]` | Mark messages as unread |
| `mail messages move [REF...]` | Move messages to a folder |
| `mail messages reply REF` | Reply to a message |
| `mail messages send` | Compose and send a message |
| `mail messages star [REF...]` | Star messages |
| `mail messages trash [REF...]` | Move messages to the trash |
| `mail messages unlabel [REF...]` | Detach a label from messages |
| `mail messages unschedule [REF...]` | Cancel a scheduled send, returning the message to drafts |
| `mail messages unstar [REF...]` | Remove the star from messages |
| `mail messages unsubscribe REF...` | Ask a mailing list to stop |
| `mail settings addresses get REF` | Show one address, including its signature |
| `mail settings addresses list` | List the addresses on the account |
| `mail settings addresses update REF` | Set an address's display name or signature |
| `mail settings autoreply disable` | Turn the auto-reply off, keeping its schedule |
| `mail settings autoreply enable` | Turn the auto-reply on, keeping its schedule |
| `mail settings autoreply get` | Show the auto-reply and its schedule |
| `mail settings autoreply set` | Configure the auto-reply and turn it on |
| `mail settings filters apply [REF...]` | Run filters over mail that is already in the mailbox |
| `mail settings filters create` | Create a filter |
| `mail settings filters delete REF...` | Delete filters |
| `mail settings filters disable REF...` | Disable filters |
| `mail settings filters enable REF...` | Enable filters |
| `mail settings filters get REF` | Show what a filter matches and does |
| `mail settings filters list` | List your filters |
| `mail settings filters reorder REF...` | Set the order filters run in |
| `mail settings filters update REF` | Change what a filter is called, matches, or does |
| `mail settings folders create` | Create a folder |
| `mail settings folders delete REF...` | Delete folders |
| `mail settings folders list` | List your folders |
| `mail settings folders update REF` | Rename or recolor a folder |
| `mail settings get` | Show the mail settings now in effect |
| `mail settings labels create` | Create a label |
| `mail settings labels delete REF...` | Delete labels |
| `mail settings labels list` | List your labels |
| `mail settings labels update REF` | Rename or recolor a label |
| `mail settings list` | List the mail settings that can be changed |
| `mail settings senders allow EMAIL...` | Always let someone reach the inbox |
| `mail settings senders block EMAIL...` | Send someone's mail straight to blocked |
| `mail settings senders forget EMAIL...` | Drop a standing decision, letting the spam filter decide again |
| `mail settings senders list` | List every standing decision about a sender |
| `mail settings senders spam EMAIL...` | Send someone's mail straight to spam |
| `mail settings set KEY VALUE` | Change one mail setting |

### `pass`

Vaults, logins and secrets

| Command | What it does |
| --- | --- |
| `pass aliases contacts allow REF CONTACT_REF...` | Let a contact's mail reach you again |
| `pass aliases contacts block REF CONTACT_REF...` | Stop a contact's mail reaching you |
| `pass aliases contacts create REF EMAIL` | Make an address that writes to somebody as the alias |
| `pass aliases contacts delete REF CONTACT_REF...` | Remove an address an alias can write to |
| `pass aliases contacts list REF` | List the addresses an alias can write to |
| `pass aliases create` | Create an alias |
| `pass aliases disable REF` | Stop receiving mail sent to an alias |
| `pass aliases enable REF` | Start receiving mail sent to an alias |
| `pass aliases list` | List your aliases |
| `pass aliases options` | List the suffixes and mailboxes an alias can use |
| `pass breaches get REF` | Show the breaches one address has appeared in |
| `pass breaches list` | List the addresses Proton watches, and how many breaches each is in |
| `pass export` | Write every vault out as a Proton Pass archive |
| `pass generate` | Make a password |
| `pass import PATH` | Read a Proton Pass archive back in |
| `pass invitations accept REF...` | Take a vault somebody offered you |
| `pass invitations decline REF...` | Turn down a vault somebody offered you |
| `pass invitations list` | List vaults other people have offered you |
| `pass items create` | Create an item |
| `pass items delete [REF...]` | Delete items permanently |
| `pass items get REF` | Show one item, decrypted |
| `pass items list` | List items across your vaults |
| `pass items pin REF...` | Keep items at the top of the list |
| `pass items revisions list REF` | Show what an item used to be |
| `pass items totp REF` | Print the current two-factor code for an item |
| `pass items trash [REF...]` | Move items to the trash |
| `pass items unpin REF...` | Stop keeping items at the top |
| `pass items update REF` | Change an item's fields |
| `pass links create REF` | Make a link that shows one item |
| `pass links list` | List the links you have made |
| `pass links revoke REF...` | Stop a link working |
| `pass settings domains list` | List the domains an alias can be made on |
| `pass settings mailboxes create EMAIL` | Add an address for aliases to forward to |
| `pass settings mailboxes delete REF` | Remove an address aliases forward to |
| `pass settings mailboxes list` | List the addresses your aliases forward to |
| `pass settings mailboxes resend REF` | Send the confirmation code again |
| `pass settings mailboxes update REF` | Change a mailbox |
| `pass settings mailboxes verify REF` | Confirm an address with the code Proton emailed it |
| `pass trash empty` | Delete everything in the trash, permanently |
| `pass trash list` | List what is in the trash |
| `pass trash restore [REF...]` | Put items back where they came from |
| `pass vaults create` | Create a vault |
| `pass vaults delete REF...` | Delete vaults, and everything in them |
| `pass vaults get REF` | Show one vault in full |
| `pass vaults list` | List your vaults |
| `pass vaults share add REF EMAIL` | Offer a vault to somebody |
| `pass vaults share list REF` | List who has been offered a vault |
| `pass vaults share remove REF EMAIL` | Withdraw an offer nobody has taken |
| `pass vaults update REF` | Rename a vault, or change how it looks |

## Account

### `account`

Your Proton account, its settings and your session

| Command | What it does |
| --- | --- |
| `account get` | Show the account, its storage and this machine's session |
| `account login` | Sign in and save the session for this profile |
| `account logout` | Discard the saved session for this profile |
| `account profiles delete REF...` | Remove saved sessions by profile name |
| `account profiles list` | List the profiles with a saved session |
| `account sessions list` | List every signed-in session |
| `account sessions revoke [REF...]` | Invalidate sessions at Proton |
| `account settings get` | Show the account settings now in effect |
| `account settings list` | List the account settings that can be changed |
| `account settings set KEY VALUE` | Change one account setting |

### `api`

Send a raw authenticated request to the Proton API

| Command | What it does |
| --- | --- |
| `api METHOD ENDPOINT` | Send a raw authenticated request to the Proton API |

## proton itself

### `changelog`

Print what each release changed

| Command | What it does |
| --- | --- |
| `changelog [VERSION]` | Print what each release changed |

### `completion`

Generate a shell completion script

| Command | What it does |
| --- | --- |
| `completion` | Generate a shell completion script |

### `uninstall`

Remove a curl/PowerShell-installed proton

| Command | What it does |
| --- | --- |
| `uninstall` | Remove a curl/PowerShell-installed proton |

### `update`

Update proton to the latest release

| Command | What it does |
| --- | --- |
| `update [VERSION]` | Update proton to the latest release |

### `version`

Print the version and build information

| Command | What it does |
| --- | --- |
| `version` | Print the version and build information |
