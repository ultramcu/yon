#!/usr/bin/env sh
# Build the macOS Yon.app bundle and register the .yon document type so that
# double-clicking a collection in Finder opens it in Yon.
#
# `fyne package` produces the bundle (icon + metadata from FyneApp.toml) but its
# Info.plist template has no document-type support, so this script injects the
# CFBundleDocumentTypes + an exported UTI afterwards with PlistBuddy.
#
# Run from the repo root:  sh packaging/macos/package.sh
set -eu

APP="Yon.app"
UTI="com.ultramcu.yon.collection"

# 1. Build the .app bundle (overwrites any previous one). The Fyne CLI version is
# pinned (not @latest) so the bundle build is reproducible and a future CLI change
# to the Info.plist template can't silently break the PlistBuddy edits below.
go run fyne.io/tools/cmd/fyne@v1.7.1 package -os darwin

PLIST="$APP/Contents/Info.plist"
if [ ! -f "$PLIST" ]; then
	echo "error: $PLIST not found (did fyne package succeed?)" >&2
	exit 1
fi

pb() { /usr/libexec/PlistBuddy -c "$1" "$PLIST"; }

# 2. Declare that Yon opens documents of our UTI, and owns the file type.
pb "Add :CFBundleDocumentTypes array"
pb "Add :CFBundleDocumentTypes:0 dict"
pb "Add :CFBundleDocumentTypes:0:CFBundleTypeName string 'Yon Collection'"
pb "Add :CFBundleDocumentTypes:0:CFBundleTypeRole string Editor"
pb "Add :CFBundleDocumentTypes:0:LSHandlerRank string Owner"
pb "Add :CFBundleDocumentTypes:0:LSItemContentTypes array"
pb "Add :CFBundleDocumentTypes:0:LSItemContentTypes:0 string $UTI"

# 3. Export the UTI: maps the .yon extension to our type (a kind of JSON).
pb "Add :UTExportedTypeDeclarations array"
pb "Add :UTExportedTypeDeclarations:0 dict"
pb "Add :UTExportedTypeDeclarations:0:UTTypeIdentifier string $UTI"
pb "Add :UTExportedTypeDeclarations:0:UTTypeDescription string 'Yon Collection'"
# Conform to public.json (it is JSON) and also public.data/public.content so
# Launch Services indexes and routes the type reliably.
pb "Add :UTExportedTypeDeclarations:0:UTTypeConformsTo array"
pb "Add :UTExportedTypeDeclarations:0:UTTypeConformsTo:0 string public.json"
pb "Add :UTExportedTypeDeclarations:0:UTTypeConformsTo:1 string public.content"
pb "Add :UTExportedTypeDeclarations:0:UTTypeConformsTo:2 string public.data"
pb "Add :UTExportedTypeDeclarations:0:UTTypeTagSpecification dict"
pb "Add :UTExportedTypeDeclarations:0:UTTypeTagSpecification:public.filename-extension array"
pb "Add :UTExportedTypeDeclarations:0:UTTypeTagSpecification:public.filename-extension:0 string yon"

# 4. Validate the patched plist before declaring success.
plutil -lint "$PLIST"

echo "Built $APP with the .yon file association."
echo "Move it to /Applications (or run 'open $APP' once) so Launch Services picks up the type."
