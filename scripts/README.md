# Scripts

Everything the maintainer or CI runs, in whatever language suits it: shell installers, a TypeScript generator, a Node publisher, a Go generator, and the demo recorder.

| Directory or file | Role |
| --- | --- |
| `install.sh`, `install.ps1` | The installers users curl; referenced from the README |
| `build-hv-helpers.sh` | Builds the CAPTCHA webview helpers that get embedded (`just build`) |
| `gen-completions.sh` | Emits the shell completions shipped in releases (a goreleaser `before` hook) |
| `gendocs/` | Generates `docs/commands/README.md` from the command tree (`just docs`) |
| `openapi-generator/` | Generates `openapi.yaml` from the WebClients TypeScript source (`just openapi`) |
| `terminal-demo/` | Records the README panel against the primary account (`just demo`) |
| `publish-npm.mjs` | Publishes the npm package on release |
| `release-helpers-check.sh` | Verifies a release's embedded helpers before publishing (a goreleaser `before` hook) |

## Command Reference Generator

Writes `docs/commands/README.md` - every command in the tree, one row each - by walking the assembled Cobra tree rather than by reading the source or the prose.

```bash
just docs
```

The prose pages beside it are hand-written; only the index is generated. That split is deliberate: generated per-command pages read badly, but an index is exactly the thing that goes stale silently when a command is renamed. CI regenerates it and fails on a diff, so a command that exists is a command that is listed, under its current name.

It shares the tree with `internal/cli/conformance_test.go`, which checks the same commands against the rules the interface is meant to obey. Both call `cli.Root()`.

## OpenAPI Generator

Auto-generates `openapi.yaml` from [ProtonMail/WebClients](https://github.com/ProtonMail/WebClients) TypeScript source files using [ts-morph](https://github.com/dsherret/ts-morph) for full AST parsing with type resolution.

### Usage

```bash
just openapi
```

This outputs `openapi.yaml` in the project root. First run clones the WebClients repo to `/tmp/proton-cli-WebClients` (~30 seconds). Subsequent runs pull updates (~1 second).

### What It Extracts

Per endpoint:

| Source | OpenAPI |
|---|---|
| Function name | `operationId`, `summary` |
| `url` property | `paths` (constants resolved from source) |
| `method` property | HTTP method |
| `data` parameter type | `requestBody` schema (types resolved through imports) |
| `params` object | `parameters` (query) |
| Template literal `${vars}` | `parameters` (path) |
| `input: 'form'\|'binary'` | Request `content-type` (`multipart/form-data`, `application/octet-stream`) |
| `output: 'stream'\|'arrayBuffer'\|'text'` | Response `content-type` |
| JSDoc comments | `description` |
| `@deprecated` tag | `deprecated: true` |
| `/** Public **/` comments, `credentials: 'omit'` | `security: []` |
| `timeout` property | `x-timeout` |
| `keepalive` property | `x-keepalive` |
| `silence` array | `x-expected-errors` |
| All exported enums | Comment block in components section |

Global:

| Source | OpenAPI |
|---|---|
| All `export enum` declarations | Enum reference comments with all values |
| All `export const = 'string'` | Used to resolve URL template constants |
| TypeScript interfaces | Resolved for request body property types, optionality, and comments |

### How It Works

1. **Clone** - shallow clones (or pulls) `ProtonMail/WebClients` into `/tmp/`
2. **Project setup** - creates a ts-morph `Project` with `tsconfig.base.json` for path resolution
3. **Registry** - scans all source files for string/number constants and enum declarations
4. **Parse** - walks all exported declarations in `api/**/*.ts`, extracts endpoint metadata from the AST
5. **Type resolution** - follows TypeScript imports to resolve `data: SomeType` to actual property lists (including `extends`, `Partial<>`, `Omit<>`, etc.)
6. **Emit** - generates OpenAPI 3.1 YAML to stdout

### File Structure

```
openapi-generator/
├── index.ts              - entry point
├── clone.ts              - git clone/pull
├── parse.ts              - ts-morph project setup, file discovery
├── registry.ts           - constant and enum collection
├── extract-endpoint.ts   - endpoint extraction from AST nodes
├── extract-params.ts     - body/query param type resolution
├── emit-yaml.ts          - OpenAPI YAML output
└── types.ts              - shared TypeScript interfaces
```
