#!/usr/bin/env bash
# Convert the game's films (assets/Konung/Video/*.flm, an old Windows video
# format) once, inside a container with Go and ffmpeg, so this machine needs
# only podman or docker. Without converted films the game skips them.
#
#   scripts/convert-films.sh            converts what is not converted yet
#   scripts/convert-films.sh -force     converts everything again
#
# Environment variables:
#   KONUNG_ASSETS=assets/Konung   the game folder (Video/ inside it)
#   KONUNG_GO_IMAGE=docker.io/library/golang:1.26
#   KONUNG_ENGINE=podman      or docker
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
assets=${KONUNG_ASSETS:-$repo/assets/Konung}
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
[[ -d $assets/Video ]] || { echo "No $assets/Video folder: nothing to convert." >&2; exit 1; }

force=""
if [[ ${1:-} == -force ]]; then
	force="-force"
fi
own=""
if [[ $engine == docker ]]; then # rootful Docker: give the files back
	own=" && chown -R $(id -u):$(id -g) /video"
fi
echo "Converting the films in $assets/Video (installs ffmpeg in the container first; takes a while) ..."
"$engine" run --rm -v "$repo:/src:ro${zl:+,$zl}" -v "$assets/Video:/video${zl:+:$zl}" \
	-v konung-gocache:/go/pkg/mod -w /src "$go_image" \
	sh -c "apt-get update -qq && apt-get install -y -qq ffmpeg >/dev/null && go run ./cmd/video-convert $force /video$own"
echo "Done."
