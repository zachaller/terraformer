#!/usr/bin/env bash
# Builds a debug APK: binds the Go game into an Android library with
# ebitenmobile, then builds the Android app around it with Gradle.
#
# Needs ANDROID_HOME (Android SDK) and ANDROID_NDK_HOME (Android NDK), plus
# ebitenmobile: go install github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@<ebiten version>
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

: "${ANDROID_HOME:?set ANDROID_HOME to your Android SDK}"
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to your Android NDK}"

ebitenmobile bind -target android -androidapi 23 -ldflags="-s -w" \
  -javapkg com.zachaller.terraformer \
  -o android/app/libs/terraformer.aar ./mobile

cd android
./gradlew assembleDebug
echo "APK: $root/android/app/build/outputs/apk/debug/app-debug.apk"
