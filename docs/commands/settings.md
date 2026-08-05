# Settings

Read your account and mail settings, and change the mail ones.

```bash
proton-cli settings get      # account settings
proton-cli settings mail     # mail settings
proton-cli settings set      # list the writable keys
```

## Changing a mail setting

```bash
proton-cli settings set view-mode 1
proton-cli settings set draft-type text/html
proton-cli settings set hide-remote-images 1
proton-cli settings set delay-send 10
```

| Key | Values |
| --- | --- |
| `page-size` | messages per page: `50`, `100`, `200` |
| `view-mode` | `0` conversations, `1` messages |
| `sign` | `0` off, `1` sign outgoing mail |
| `attach-public-key` | `0`, `1` |
| `auto-save-contacts` | `0`, `1` |
| `hide-remote-images` | `0`, `1` |
| `hide-embedded-images` | `0`, `1` |
| `draft-type` | `text/html`, `text/plain` |
| `pm-signature` | `0` off, `1` on |
| `show-moved` | `0` to `3` |
| `shortcuts` | `0`, `1` |
| `sticky-labels` | `0`, `1` |
| `prompt-pin` | `0`, `1` |
| `enable-folder-color` | `0`, `1` |
| `delay-send` | undo-send window in seconds, `0` to `20` |
| `almost-all-mail` | `0`, `1` |
