# Releasing

Determine the next release version. Note that we automatically release on go-sdk
bumps, doing patch tags.

1. **Update `CHANGELOG.md`.** Add all the changes since the last release, matching
   the pattern in the file. Don't include go-sdk bumps.
2. **Make a PR and merge it.** Branch protection requires a PR, you can self-merge.
3. **Tag the commit** and push it:

   ```bash
   git checkout master && git pull
   git tag v0.2.0
   git push origin v0.2.0
   ```

   Make sure to pull latest before tagging.

4. **Watch Actions → Release.** It's a good idea to check that it finished, but
   there's no reason to think it won't.

## What the tag sets off

`release.yml` runs goreleaser, which:

- cross-compiles for linux, darwin and windows on amd64 and arm64
- creates the GitHub release with those archives, shell completions and checksums
- writes release notes from the commit subjects since the last tag
- commits an updated formula to
  [`incident-io/homebrew-tap`](https://github.com/incident-io/homebrew-tap), which
  is what `brew install incident-io/tap/inc` reads

## Version numbers

Patch for fixes, minor for new commands or flags, major for anything that breaks
an existing invocation.

## Rules on tags

A ruleset named `release-tags` blocks deleting, moving and force-updating `v*`
tags, because a published release's download URLs are baked into the Homebrew
formula and into anything pinned to that version. Creating a tag is unrestricted.

If you tag the wrong commit, cut a new version.

## Checking it worked

```bash
gh release view v0.2.0                 # archives and checksums present
brew update && brew upgrade inc        # formula picked up the new version
```

The formula commit appears in the tap's history as "Brew formula update for inc
version v0.2.0".

## Local dry run

`make snapshot` runs the whole goreleaser pipeline without publishing anything or
needing a tag. Worth doing after changing `.goreleaser.yml`.
