#!/usr/bin/env bash
# Prints the game's version: versionName in android/app/build.gradle, the one
# place it's kept.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
sed -n 's/^ *versionName "\(.*\)"/\1/p' "$root/android/app/build.gradle"
