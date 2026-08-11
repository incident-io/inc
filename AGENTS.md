# AGENTS.md — Using `inc` (incident.io CLI)

`inc` is a CLI for the [incident.io](https://incident.io) API. It manages incidents, alerts, catalog entries, escalations, schedules, and more. Every API command supports `--output json` for machine-readable output (`config` and `auth login|token` print plain text).

## Authentication

Set the `INCIDENT_API_KEY` environment variable. That's it.

```bash
export INCIDENT_API_KEY=inc_abc123...
```

Verify it works:

```bash
inc auth status
```

## Command Grammar

```
inc <resource> <action> [flags]
```

Resources are plural nouns. Actions are usually verbs (exceptions: `schedules entries`, `post-mortems content`, `auth status|token`). Examples:

```bash
inc incidents list --status-category live
inc incidents show INC-123
inc catalog entries list --type-id 01HXYZ
inc escalations create --title "Database latency spike" --escalation-path-id 01HXYZ
```

## Output

Always use `--output json` when calling `inc`. Never parse table output.

```bash
# Full JSON
inc incidents list --output json

# Filter with --jq to reduce token usage (typed commands return unwrapped arrays)
inc incidents list --output json --jq '.[] | {id, name}'

# Limit fields to save tokens
inc incidents list --output json --fields id,name,reference,incident_status,severity

# Note: `inc api` returns raw enveloped responses, so use .incidents[] there:
inc api GET /v2/incidents --output json --jq '.incidents[] | {id, name}'
```

## The `inc api` Escape Hatch

`inc api` can hit any incident.io API endpoint directly. Use it when a dedicated command doesn't exist yet, or when you need fine-grained control.

```bash
# GET
inc api GET /v2/incidents --jq '.incidents[] | {id, name}'

# GET with query params
inc api GET /v2/incidents --field status[one_of]=live --field page_size=10

# POST with flags
inc api POST /v2/incidents --field name="Database outage" --field severity_id="01HXYZ"

# POST with stdin
echo '{"name": "Database outage"}' | inc api POST /v2/incidents --input -

# Paginate through all results
inc api GET /v2/incidents --paginate --jq '.incidents[]'
```

## Key Commands

| Command | Description |
|---------|-------------|
| `inc incidents list` | List incidents. Filter with `--status-category`, `--severity-id`, `--sort-by`. |
| `inc incidents show ID` | Get a single incident. `ID` is a ULID or a reference (`INC-123`). |
| `inc incidents create` | Create an incident. |
| `inc incidents update ID` | Update an incident. Use `--notify=false` for silent updates. `ID` accepts a reference. |
| `inc incidents close ID` | Close an incident. `ID` accepts a reference. |
| `inc alerts list` | List alerts. |
| `inc alerts show ID` | Get a single alert. |
| `inc catalog types list` | List catalog types. |
| `inc catalog types show ID` | Get a single catalog type. |
| `inc catalog types create` | Create a catalog type. Requires `--name`, `--description`. |
| `inc catalog types delete ID` | Delete a catalog type. |
| `inc catalog entries list --type-id TYPE` | List catalog entries of a given type. |
| `inc catalog entries show ID` | Get a single catalog entry. |
| `inc catalog entries create` | Create a catalog entry. Requires `--type-id`, `--name`. |
| `inc catalog entries update ID` | Update a catalog entry. Requires `--name`. |
| `inc catalog entries delete ID` | Delete a catalog entry. |
| `inc escalations list` | List live escalations. |
| `inc escalations show ID` | Get a single escalation. |
| `inc escalations create` | Trigger an escalation. Requires `--title`. |
| `inc escalations paths list` | List escalation path configurations. |
| `inc escalations paths show ID` | Get a single escalation path. |
| `inc schedules list` | List on-call schedules. |
| `inc schedules show ID` | Get a single schedule. |
| `inc schedules entries ID` | List who is on call. Use `--from` and `--until` for time window. |
| `inc schedules override` | Create a schedule override (swap a shift). |
| `inc severities list` | List severities. |
| `inc severities show ID` | Get a single severity. |
| `inc custom-fields list` | List custom fields. |
| `inc custom-fields show ID` | Get a single custom field. |
| `inc users list` | List users. Filter with `--email`. |
| `inc users show ID` | Get a single user. |
| `inc roles list` | List incident roles. |
| `inc roles show ID` | Get a single incident role. |
| `inc incident-updates list` | List incident updates. Filter with `--incident-id`. |
| `inc post-mortems list` | List post-mortem documents. Filter with `--incident-id`. |
| `inc post-mortems show ID` | Get a single post-mortem document. |
| `inc post-mortems content ID` | Get the full post-mortem content as markdown. |
| `inc follow-ups list` | List follow-up actions. Filter with `--incident-id`. |
| `inc follow-ups show ID` | Get a single follow-up. |
| `inc auth login` | Save your API key. |
| `inc auth status` | Check if authentication is working. |
| `inc auth token` | Print the current API key to stdout. |
| `inc config set KEY VALUE` | Set a config value. |
| `inc config get KEY` | Get a config value. |
| `inc config list` | List all config values. |
| `inc describe` | Output JSON schema of all commands and flags for agent discovery. |
| `inc api METHOD PATH` | Hit any API endpoint directly. |

## Pagination

List commands auto-paginate when output is piped — they return all results across all pages. On a TTY with no `--limit`, they stop at 25 results (one page) and print a notice to stderr when more exist; pass `--limit 0` for everything.

- `--limit N` — Stop after N total results.
- `--page-size N` — Control batch size per API call (default 25, max 250).
- On `inc api`, pass `--paginate` to opt into auto-pagination. With `--jq`, the filter is applied to each page's response envelope in turn (like `gh api --paginate`); without it, multiple pages are printed as a JSON array of page objects.

## Error Handling

Exit codes:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | User error (bad flags, missing auth) |
| `2` | API error (4xx/5xx from incident.io) |
| `3` | Network error |

Errors follow the same format resolution as data: piped stdout gets JSON errors on stderr without any flag, an explicit `--output` wins either way. A real 401 looks like:

```json
{
  "error": "http_401",
  "message": "Unauthenticated",
  "suggestion": "Run 'inc auth login' to set a new API key.",
  "request_id": "yEPGHT_A",
  "retryable": false,
  "api_error": {
    "type": "authentication_error",
    "status": 401,
    "request_id": "yEPGHT_A",
    "errors": [{"code": "unauthenticated", "message": "Unauthenticated"}]
  }
}
```

The top-level fields are the CLI's stable interface; `api_error` is the API's response body verbatim, so any detail the server sends (error codes, debug info in dev environments) survives untouched. Include `request_id` when reporting problems to incident.io.

The `error` values are `http_<status>` for API errors, `network_error`, `user_error`, `invalid_request`, `read_error`, and `error` for anything else. Check the `retryable` field before retrying failed requests.

## Tips for Agents

1. **Run `inc describe` first.** It outputs a full JSON schema of every command and flag.
2. **Always pass `--output json`.** Table output is for humans.
3. **Use `--jq` to filter responses.** Less data means fewer tokens. `--jq` implies JSON output, so you don't need `--output json` alongside it.
4. **Use `--fields` to limit response size.** Only request the fields you need.
5. **Use `inc api` when a dedicated command doesn't exist.** It can do anything the API can do.
6. **Use `--dry-run` to preview the HTTP request** before sending it (works on reads too). The preview JSON prints to stderr, nothing is sent, and the exit code is 0.
7. **Check `retryable` on errors.** Don't retry non-retryable errors.
8. **Don't prompt.** `inc` never prompts when stdout is not a TTY. All inputs must be passed as flags or stdin.
9. **Rate limit is 1,200 req/min.** Every command retries 429s automatically (up to 3 attempts, respecting `Retry-After`). If retries are exhausted you get a structured error with `"retryable": true` and exit code 2. Network errors are only retried for read requests, never for mutations.
10. **Mutations generate random idempotency keys.** `incidents create` and `escalations create` use a fresh UUID each call. Retrying after a timeout will create duplicates — check if the resource was created before retrying.
11. **Errors go to stderr, data to stdout.** Pipe stdout for data, capture stderr for errors. `--quiet` suppresses stdout on success.
12. **Piped output defaults to JSON.** When stdout is not a TTY, the output format auto-switches to JSON. Use `--output table` to force table output in a pipe.

## API Coverage

`inc` wraps the incident.io API (V1 for severities and post-mortems, V2 for most resources, V3 for catalog). Full API documentation is at [api-docs.incident.io](https://api-docs.incident.io). If a resource isn't available as a dedicated command, use `inc api` to access it directly.
