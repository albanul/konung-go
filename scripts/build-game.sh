#!/usr/bin/env bash
# Build the desktop game (cmd/konung) inside a Go container, so this machine
# needs only podman or docker, not Go.
#
#   scripts/build-game.sh            the game for this computer's system
#   scripts/build-game.sh windows    output/game/windows/konung.exe
#   scripts/build-game.sh mac        output/game/mac/konung-apple-silicon and konung-intel
#   scripts/build-game.sh linux      output/game/linux/konung (64-bit PC)
#   scripts/build-game.sh all        all of them
#
# The game looks for the game files in assets/Konung next to the program or
# in a folder above it, so it runs from output/game/... in this repository.
# To play on another computer, copy the program together with assets/Konung:
#
#   some-folder/konung.exe
#   some-folder/assets/Konung/...   (the installed game)
#
# Environment variables:
#   KONUNG_GO_IMAGE=docker.io/library/golang:1.26   the Go image
#   KONUNG_ENGINE=podman      or docker
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
go_image=${KONUNG_GO_IMAGE:-docker.io/library/golang:1.26}
engine=${KONUNG_ENGINE:-}
if [[ -z $engine ]]; then
	if command -v podman >/dev/null; then engine=podman
	elif command -v docker >/dev/null; then engine=docker
	else echo "Need podman or docker (see README.md)." >&2; exit 1
	fi
fi
zl=""
if [[ -r /sys/fs/selinux/enforce && $(cat /sys/fs/selinux/enforce) == 1 ]]; then
	zl=Z
fi

say() { printf '%s\n' "$*"; }

# run_go runs a shell command in the Go image with the repository at /src;
# Go's downloads are kept in the volume konung-gocache.
run_go() {
	local own=""
	if [[ $engine == docker ]]; then # rootful Docker: give the files back
		own=" && chown -R $(id -u):$(id -g) /src/output/game"
	fi
	"$engine" run --rm -v "$repo:/src${zl:+:$zl}" -v konung-gocache:/go/pkg/mod -w /src \
		"$go_image" sh -c "$1$own"
}

windows() {
	say "Building output/game/windows/konung.exe ..."
	run_go "CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o output/game/windows/konung.exe ./cmd/konung"
}

mac() {
	say "Building output/game/mac/konung-apple-silicon and konung-intel ..."
	run_go "CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o output/game/mac/konung-apple-silicon ./cmd/konung &&
		CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o output/game/mac/konung-intel ./cmd/konung"
}

linux() {
	# No C needed: Ebitengine loads the system's graphics and sound libraries
	# when the game starts.
	say "Building output/game/linux/konung ..."
	run_go "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o output/game/linux/konung ./cmd/konung"
}

target=${1:-}
if [[ -z $target ]]; then
	case $(uname -s) in
	Darwin) target=mac ;;
	Linux) target=linux ;;
	*) target=windows ;;
	esac
fi
mkdir -p "$repo/output/game"
case $target in
windows) windows ;;
mac) mac ;;
linux) linux ;;
all) windows; mac; linux ;;
*)
	sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
say "Done. The programs are in output/game (see README.md, «Desktop game»)."
