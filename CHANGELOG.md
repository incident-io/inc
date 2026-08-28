# Changelog

We automatically cut new releases for `sdk-go` bumps, so this changelog only
lists out non-automated changes.

## 0.4.0 - 2026-08-28

- Allow configuring `app_url`
- Support unsetting configuration options, to fall back to defaults.
- Show single item results vertically, eg `inc incidents show <id>`
- Introduce curated fields for single item results, setting nice defaults.

## 0.3.0 - 2026-08-28

- `inc auth login` now triggers the OAuth login flow by default. When authenticated
  as a user, all permission checks are made against the user itself instead of an
  API key, and all actions are taken as the user.
- Bump go version to 1.26.7

## 0.2.0 - 2026-08-18

- `incidents show`, `incidents update` and `incidents close` accept a reference
  (`INC-123`) as well as an ID.
- `--limit 0` returns everything again. It was being read as "no flag given" and
  silently capped.
- Tables fit the terminal width instead of wrapping columns onto later lines.
- Status and severity are coloured on a terminal, and left alone when piped.
- Timestamps read as ages (`5m ago`, `3d ago`) on a terminal, and stay as
  RFC3339 when piped so scripts can parse them.
- List commands show a curated set of columns by default instead of every field
  the API returns. `--fields` still overrides, and JSON output is unfiltered.

## 0.1.1 — 2026-07-23

- Documented `mise` as an installation option.
- Added a security policy.
- Built with Go 1.26.5.

## 0.1.0 — 2026-07-17

- First release.
