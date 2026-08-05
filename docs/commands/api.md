# Raw API

`proton-cli api` sends an authenticated request to any Proton endpoint, using the session you already have. It's the escape hatch for anything the high-level commands don't cover.

```bash
proton-cli api GET /drive/volumes
proton-cli api GET /calendar/v1
proton-cli api GET /mail/v4/messages --query Page=0 --query PageSize=10
proton-cli api POST /calendar/v1 --body '{"Name":"Work","Color":"#7272a7","Display":1,"AddressID":"..."}'
proton-cli api PUT /mail/v4/settings/viewmode --body '{"ViewMode":1}'
proton-cli api DELETE /mail/v4/labels/LABEL_ID
```

| Flag | Purpose |
| --- | --- |
| `--body JSON` | Request body |
| `--query key=value` | Query parameter, repeatable |

Combine it with `jq` like any other command:

```bash
proton-cli api GET /calendar/v1 --output json | jq -r '.Calendars[].ID'
```

## Caveats

Responses come back as the API returns them, in Proton's `PascalCase`, and encrypted fields stay encrypted: this command does no key handling. If you want decrypted content, use the service commands.

## Endpoint reference

[`openapi.yaml`](../../openapi.yaml) in the repository root documents roughly 740 endpoints, generated from Proton's own web client source. It covers paths, methods, request and response schemas, and query parameters.

The generator lives in [`scripts/`](../../scripts/README.md) and a weekly workflow keeps the spec in sync with upstream.
