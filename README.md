# inc

A command-line interface for the [incident.io](https://incident.io) API. Manages incidents, alerts, catalog entries, escalations, schedules, and more.

Built for both humans and LLM agents — every command supports `--output json` for machine-readable output.

## Install

```bash
# Homebrew
brew install incident-io/tap/inc

# mise
mise use -g ubi:incident-io/inc
```

Or download a binary from [GitHub Releases](https://github.com/incident-io/inc/releases).

Shell completions for bash, zsh, and fish install automatically with brew, or generate them yourself with `inc completion <shell>` (e.g. `inc completion zsh > ~/.zfunc/_inc`).

## Authentication

```bash
# Option 1: environment variable
export INCIDENT_API_KEY=inc_abc123...

# Option 2: save to config
inc auth login
```

Verify it works:

```bash
inc auth status
```

## Usage

```bash
# List live incidents
inc incidents list --status-category live

# Show a single incident, by ULID or reference
inc incidents show INC-123

# JSON output with jq filtering
inc incidents list --output json --jq '.[] | {id, name, status: .incident_status.name}'

# Limit fields and results
inc incidents list --limit 10 --fields id,name,reference

# Create an incident
inc incidents create --name "Database outage" --visibility public --severity-id 01HXYZ

# Update an incident
inc incidents update 01HXYZ --name "Database outage (resolved)"

# Dry-run a mutation (prints request without sending)
inc incidents create --name "Test" --visibility public --severity-id 01HXYZ --dry-run

# Hit any API endpoint directly
inc api GET /v2/incidents --field 'status_category[one_of]=live' --jq '.incidents[].name'

# POST with flags
inc api POST /v2/incidents --field name="Outage" --field visibility=public

# POST with stdin
echo '{"name": "Outage", "visibility": "public"}' | inc api POST /v2/incidents --input -

# Auto-paginate raw API calls
inc api GET /v2/incidents --paginate --jq '.incidents[]'
```

## Commands

| Command                                                  | Description                                   |
| -------------------------------------------------------- | --------------------------------------------- |
| `inc incidents list\|show\|create\|update\|close`        | Manage incidents.                             |
| `inc alerts list\|show`                                  | Manage alerts.                                |
| `inc catalog types list\|show\|create\|delete`           | Manage catalog types.                         |
| `inc catalog entries list\|show\|create\|update\|delete` | Manage catalog entries.                       |
| `inc escalations list\|show\|create`                     | Manage live escalations.                      |
| `inc escalations paths list\|show`                       | Manage escalation path configs.               |
| `inc schedules list\|show\|entries\|override`            | Manage on-call schedules.                     |
| `inc severities list\|show`                              | List severities.                              |
| `inc users list\|show`                                   | List users.                                   |
| `inc roles list\|show`                                   | List incident roles.                          |
| `inc custom-fields list\|show`                           | List custom fields.                           |
| `inc post-mortems list\|show\|content`                   | Post-mortem documents.                        |
| `inc follow-ups list\|show`                              | Follow-up actions.                            |
| `inc incident-updates list`                              | Incident update history.                      |
| `inc config set\|get\|list`                              | Manage CLI config.                            |
| `inc describe`                                           | JSON schema of all commands (for LLM agents). |
| `inc api METHOD PATH`                                    | Hit any API endpoint directly.                |
| `inc auth login\|status\|token`                          | Manage authentication.                        |

## Output

```bash
# Human-readable table (default in terminal)
inc incidents list --fields id,name,reference

# JSON (default when piped)
inc incidents list --output json

# JSON with jq filter
inc incidents list --output json --jq '.[] | {id, name}'

# Limit fields
inc incidents list --output json --fields id,name

# Suppress output (scripting)
inc incidents close 01HXYZ --quiet
```

When stdout is piped, output automatically switches to JSON. Use `--output table` to force table output in a pipe. `--jq` implies JSON output, so it works without `--output json`.

To make JSON the default in your terminal too, run `inc config set default_output json` or set `INCIDENT_DEFAULT_OUTPUT=json`.

## The `inc api` Escape Hatch

`inc api` can hit any incident.io API endpoint directly with auth pre-configured. Use it when a dedicated command doesn't exist yet.

```bash
inc api GET /v2/catalog_types --jq '.catalog_types[] | {id, name}'
inc api GET /v2/schedules --jq '.schedules[] | .name'
inc api GET /v2/alerts --paginate --jq '.alerts[].title'
```

Full API docs: [api-docs.incident.io](https://api-docs.incident.io)

## For LLM Agents

`inc` is designed to support LLM agent consumption. Run `inc describe` to get a full JSON schema of every command and flag:

```bash
inc describe                    # Full schema
inc describe incidents.list     # Single command
```

Rate-limited requests (429) are retried automatically with `Retry-After` honored. Errors are structured JSON when output resolves to JSON, with a `retryable` field to gate retries on, a `request_id` for support, and the API's full error response under `api_error`.

See [AGENTS.md](AGENTS.md) for detailed agent integration guidance.

## Development

**Prerequisites:** Go is managed via [mise](https://mise.jdx.dev/). Install mise, then:

```bash
git clone https://github.com/incident-io/inc
cd inc
mise install        # Installs the correct Go version
make build
./bin/inc --help
```

```bash
make build          # Build binary to bin/inc
make test           # Run tests
make lint           # Run golangci-lint
make fmt            # Format code
make snapshot       # Test the release pipeline locally (no publishing)
```

## Releasing

Pushing a `v*` tag publishes a release. See [RELEASING.md](RELEASING.md).

## License

MIT
