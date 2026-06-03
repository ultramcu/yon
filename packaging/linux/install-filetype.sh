#!/usr/bin/env sh
# Install the .yon file association for the current user on Linux: register the
# application/x-yon MIME type and a Yon .desktop launcher, then refresh the
# desktop databases. Double-clicking a .yon file in a file manager will open it
# in Yon (the path is passed via the .desktop Exec=yon %F line).
#
# Assumes `yon` is on PATH (e.g. copied into ~/.local/bin). Run from this folder:
#   sh install-filetype.sh
set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
install -Dm644 "$HERE/application-x-yon.xml" "$DATA/mime/packages/application-x-yon.xml"
install -Dm644 "$HERE/yon.desktop" "$DATA/applications/yon.desktop"

# Refresh the MIME + desktop caches (best effort; tools may be absent).
update-mime-database "$DATA/mime" 2>/dev/null || true
update-desktop-database "$DATA/applications" 2>/dev/null || true

echo "Installed .yon association for the current user."
echo "Ensure 'yon' is on your PATH (e.g. ~/.local/bin/yon) so the launcher can find it."
