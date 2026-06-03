#!/usr/bin/env sh
# Build the macOS Yon.app bundle and register the .yon document type so that
# double-clicking a collection in Finder opens it in Yon.
#
# `fyne package` produces the bundle (icon + metadata from FyneApp.toml) but its
# Info.plist template has no document-type support, so register-filetype.sh
# injects the CFBundleDocumentTypes + exported UTI afterwards. The release CI
# workflow uses the same register-filetype.sh, so local and CI bundles match.
#
# Run from the repo root:  sh packaging/macos/package.sh
set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
APP="Yon.app"

# 1. Build the .app bundle (overwrites any previous one). The Fyne CLI version is
# pinned (not @latest) so the bundle build is reproducible and a future CLI change
# to the Info.plist template can't silently break the PlistBuddy edits.
go run fyne.io/tools/cmd/fyne@v1.7.1 package -os darwin

# 2. Inject the .yon file association into the bundle's Info.plist.
sh "$HERE/register-filetype.sh" "$APP"

echo "Built $APP with the .yon file association."
echo "Move it to /Applications (or run 'open $APP' once) so Launch Services picks up the type."
