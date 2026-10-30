#!/usr/bin/env bash
# Builds the Android app: binds the Go game into an Android library with
# ebitenmobile, then builds the Android app around it with Gradle.
#
#   scripts/build-android.sh          debug APK
#   scripts/build-android.sh release  signed release APK and AAB (Play Store);
#                                     needs TERRAFORMER_KEYSTORE and friends,
#                                     see android/app/build.gradle
#
# Needs ANDROID_HOME (Android SDK) and ANDROID_NDK_HOME (Android NDK), plus
# ebitenmobile: go install github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@<ebiten version>
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
mode="${1:-debug}"

: "${ANDROID_HOME:?set ANDROID_HOME to your Android SDK}"
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to your Android NDK}"
if [[ "$mode" == release ]]; then
  : "${TERRAFORMER_KEYSTORE:?set TERRAFORMER_KEYSTORE to the upload keystore to sign a release}"
fi

ebitenmobile bind -target android -androidapi 23 -ldflags="-s -w" \
  -javapkg com.zachaller.terraformer \
  -o android/app/libs/terraformer.aar ./mobile

cd android
case "$mode" in
  debug)
    ./gradlew assembleDebug
    echo "APK: $root/android/app/build/outputs/apk/debug/app-debug.apk"
    ;;
  release)
    ./gradlew assembleRelease bundleRelease
    echo "APK: $root/android/app/build/outputs/apk/release/app-release.apk"
    echo "AAB: $root/android/app/build/outputs/bundle/release/app-release.aab"
    ;;
  *)
    echo "unknown mode $mode: use debug or release" >&2
    exit 2
    ;;
esac
