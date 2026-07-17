#!/bin/sh
# Generates shell completion scripts for packaging into release archives.
# Called by goreleaser (see .goreleaser.yml before hooks).
set -e
cd "$(dirname "$0")/.."
rm -rf completions
mkdir completions
for sh in bash zsh fish; do
	go run . completion "$sh" >"completions/inc.$sh"
done
