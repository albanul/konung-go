#!/usr/bin/env bash
# Build ready-to-run bundles of the game and the server for every system
# into dist/ (the GitHub release workflow runs this; it works locally too
# where Go is installed).
#
#   scripts/bundle.sh            all bundles
#   VERSION=v1.2 scripts/bundle.sh   names them konung-v1.2-<system>.zip/.tar.gz
#
# A bundle: the game, the server, the browser game's page (web/), an empty
# assets/Konung/ for the game files, the settings example and README.txt.
# The game files themselves are never included (they belong to the game's
# rights holders): the player copies them in once.
#
# Everything cross-compiles without a C compiler: Ebitengine and its sound
# library load the system's graphics and sound libraries at run time.
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"
version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
dist=$repo/dist
work=$dist/work
rm -rf "$dist"
mkdir -p "$work"

say() { printf '%s\n' "$*"; }

# The browser game's page, the same for every bundle (no game files: the
# server serves them from assets/Konung).
say "Building the browser game's page ..."
go run ./cmd/web-build -page-only -out "$work/web" >/dev/null

# bundle <system> <goos> <goarch> <game: yes/no>
bundle() {
	local name=$1 goos=$2 goarch=$3 game=$4
	local dir=$work/konung-$version-$name ext=""
	[[ $goos == windows ]] && ext=.exe
	say "Bundling $name ..."
	mkdir -p "$dir/assets/Konung"
	if [[ $game == yes ]]; then
		local ld="-s -w"
		[[ $goos == windows ]] && ld="-s -w -H=windowsgui" # no console window
		CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags="$ld" -o "$dir/konung$ext" ./cmd/konung
	fi
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags="-s -w" -o "$dir/konung-server$ext" ./cmd/konung-server
	cp -r "$work/web" "$dir/web"
	cp konung-server.conf.example "$dir/"
	cp scripts/bundle-README.txt "$dir/README.txt"
	cp scripts/bundle-assets-README.txt "$dir/assets/Konung/PUT-THE-GAME-FILES-HERE.txt"
	if [[ $game != yes ]]; then
		printf '\nThis bundle has the server only (no desktop game for this system).\n' >>"$dir/README.txt"
	fi
	(
		cd "$work"
		if [[ $goos == linux ]]; then
			tar -czf "$dist/konung-$version-$name.tar.gz" "konung-$version-$name"
		else
			if command -v zip >/dev/null; then
				zip -qr "$dist/konung-$version-$name.zip" "konung-$version-$name"
			else # Python's zip keeps the programs' execute permission too
				python3 -m zipfile -c "$dist/konung-$version-$name.zip" "konung-$version-$name"
			fi
		fi
	)
}

bundle windows windows amd64 yes
bundle macos-apple-silicon darwin arm64 yes
bundle macos-intel darwin amd64 yes
bundle linux linux amd64 yes
bundle linux-arm64 linux arm64 yes
rm -rf "$work"
say "Bundles in dist/:"
ls -la "$dist"
