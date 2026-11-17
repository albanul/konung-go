#!/usr/bin/env bash
# Run the Konung dedicated server (cmd/konung-server) in a container.
#
#   scripts/konung-server.sh start     build the image if needed, create and start the server
#   scripts/konung-server.sh update    rebuild the image and restart (accounts are kept)
#   scripts/konung-server.sh stop      stop the server
#   scripts/konung-server.sh restart   stop and start again (applies changed settings)
#   scripts/konung-server.sh logs      follow the server log (Ctrl+C to leave)
#   scripts/konung-server.sh status    show whether it runs, and the data volume
#   scripts/konung-server.sh build     only build the image
#   scripts/konung-server.sh reset     delete the server AND its data (all accounts); asks first
#
# Settings (environment variables), applied at every start:
#   KONUNG_NAME=              the server's name in the game's server list, e.g.
#                             KONUNG_NAME="Мой сервер" scripts/konung-server.sh restart
#                             (default: line 2 of the SERVER file, else
#                             «Стандартный Сервер с Командной игрой»)
#   KONUNG_PORT=7777          host port for the game (TCP) and LAN discovery (UDP)
#   KONUNG_ASSETS=assets/Konung   folder with GAME.RES (required) and SERVER (optional)
#   KONUNG_NETWORK=           set to "host" to use the host's network directly; LAN
#                             discovery (UDP broadcast) may not pass a port mapping
#   KONUNG_SERVER_ARGS=       extra server flags, e.g. '-minutes 60 -max 8'
#   KONUNG_IMAGE=konung-server  KONUNG_CONTAINER=konung-server  KONUNG_VOLUME=konung-data
#   KONUNG_ENGINE=podman      or docker
#
# The data volume holds konung.db (accounts and progress), GAME.RES and SERVER.
# GAME.RES and SERVER are copied in at every start, so edits to them apply on
# restart; the database is never touched by this script except by "reset".
# The container itself holds nothing: it is re-created at every start.
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
port=${KONUNG_PORT:-7777}
assets=${KONUNG_ASSETS:-$repo/assets/Konung}
network=${KONUNG_NETWORK:-}
image=${KONUNG_IMAGE:-konung-server}
container=${KONUNG_CONTAINER:-konung-server}
volume=${KONUNG_VOLUME:-konung-data}
engine=${KONUNG_ENGINE:-}
if [[ -z $engine ]]; then
	if command -v podman >/dev/null; then engine=podman
	elif command -v docker >/dev/null; then engine=docker
	else echo "Need podman or docker." >&2; exit 1
	fi
fi

say() { printf '%s\n' "$*"; }
die() { printf 'Error: %s\n' "$*" >&2; exit 1; }

exists() { "$engine" container inspect "$container" >/dev/null 2>&1; }
running() { [[ $("$engine" container inspect -f '{{.State.Running}}' "$container" 2>/dev/null) == true ]]; }
have_image() { "$engine" image inspect "$image" >/dev/null 2>&1; }

build() {
	say "Building image $image from $repo ..."
	"$engine" build -t "$image" -f "$repo/Containerfile" "$repo"
}

# copy_data puts GAME.RES (and SERVER, if present) into the volume through the
# created container.
copy_data() {
	[[ -f $assets/GAME.RES ]] || die "$assets/GAME.RES not found (set KONUNG_ASSETS); without it nobody can enter the world"
	"$engine" cp "$assets/GAME.RES" "$container:/data/GAME.RES"
	if [[ -f $assets/SERVER ]]; then
		"$engine" cp "$assets/SERVER" "$container:/data/SERVER"
	fi
}

create() {
	local net=(-p "$port:7777/tcp" -p "$port:7777/udp")
	local args=()
	if [[ $network == host ]]; then
		net=(--network host)
		args+=(-port "$port")
	fi
	if [[ -n ${KONUNG_NAME:-} ]]; then
		args+=(-name "$KONUNG_NAME")
	fi
	if [[ -n ${KONUNG_SERVER_ARGS:-} ]]; then
		# shellcheck disable=SC2206 # split like a command line on purpose
		eval "args+=($KONUNG_SERVER_ARGS)"
	fi
	"$engine" volume inspect "$volume" >/dev/null 2>&1 || "$engine" volume create "$volume" >/dev/null
	"$engine" create --name "$container" --restart unless-stopped \
		"${net[@]}" -v "$volume:/data" "$image" "${args[@]}" >/dev/null
}

start() {
	have_image || build
	if running; then
		say "$container is already running."
		status
		return
	fi
	# Re-create so the settings above apply; the data stays in the volume.
	if exists; then
		"$engine" rm -f "$container" >/dev/null
	fi
	say "Creating $container (port $port, data volume $volume${KONUNG_NAME:+, name «$KONUNG_NAME»}) ..."
	create
	copy_data
	"$engine" start "$container" >/dev/null
	sleep 1
	if ! running; then
		"$engine" logs "$container" >&2 || true
		die "$container stopped right after starting (log above)"
	fi
	say "Started. Players connect to this machine's address, port $port."
	"$engine" logs "$container" 2>&1 | tail -n 5
}

stop() {
	if running; then
		"$engine" stop "$container" >/dev/null
		say "Stopped $container."
	else
		say "$container is not running."
	fi
}

update() {
	build
	stop
	start
}

status() {
	if running; then
		say "$container: running ($("$engine" container inspect -f '{{.State.StartedAt}}' "$container"))"
	elif exists; then
		say "$container: stopped"
	else
		say "$container: not created"
	fi
	if "$engine" volume inspect "$volume" >/dev/null 2>&1; then
		say "data volume: $volume ($("$engine" volume inspect -f '{{.Mountpoint}}' "$volume"))"
	else
		say "data volume: $volume (not created)"
	fi
}

reset() {
	say "This deletes the container $container and the volume $volume: every account and all progress."
	read -r -p "Type 'delete' to confirm: " answer
	[[ $answer == delete ]] || { say "Nothing deleted."; return; }
	exists && "$engine" rm -f "$container" >/dev/null
	"$engine" volume rm "$volume" >/dev/null 2>&1 || true
	say "Deleted."
}

case ${1:-} in
build) build ;;
start) start ;;
stop) stop ;;
restart) stop; start ;;
update) update ;;
logs) "$engine" logs -f "$container" ;;
status) status ;;
reset) reset ;;
*)
	sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
