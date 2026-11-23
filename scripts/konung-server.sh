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
#   scripts/konung-server.sh web       (re)build the browser game (output/web) in a Go container
#   scripts/konung-server.sh reset     delete the server AND its data (all accounts); asks first
#
# The server also serves the browser game: "start" builds it on the first run
# (output/web, a few minutes) and then http://<this machine>:<port>/ is the
# game, for anyone on the network. "update" rebuilds it with the server.
#
# Server settings (name, port, players, session length, speed, team damage)
# live in konung-server.conf in the repo root: copy konung-server.conf.example
# and edit it; it is copied into the server at every start, so "restart"
# applies changes.
#
# Environment variables, applied at every start (they win over the file):
#   KONUNG_CONFIG=konung-server.conf   the settings file to use
#   KONUNG_NAME=              the server's name, e.g.
#                             KONUNG_NAME="Мой сервер" scripts/konung-server.sh restart
#   KONUNG_PORT=              host port for the game (TCP) and LAN discovery (UDP);
#                             default: "port" in the settings file, else 7777
#   KONUNG_ASSETS=assets/Konung   folder with GAME.RES (required), SERVER (optional),
#                             konung.exe + GAME.0–GAME.2 (fights between players) and
#                             MAPS.RES, GRAPH.RES, OBJECTS.RES (the world on the server),
#                             questr.RES (dialogues and trade)
#   KONUNG_NETWORK=           set to "host" to use the host's network directly; LAN
#                             discovery (UDP broadcast) may not pass a port mapping
#   KONUNG_SERVER_ARGS=       extra server flags, e.g. '-minutes 60 -max 8 -speed 1 -team-damage=false'
#   KONUNG_WEB=output/web     the browser game's folder; "off" to not build or serve it
#   KONUNG_GO_IMAGE=docker.io/library/golang:1.26   the Go image that builds the browser game
#   KONUNG_IMAGE=konung-server  KONUNG_CONTAINER=konung-server  KONUNG_VOLUME=konung-data
#   KONUNG_ENGINE=podman      or docker
#
# The data volume holds konung.db (accounts and progress) and the game files
# above, copied in at every start, so edits to them apply on restart; the
# database is never touched by this script except by "reset".
# The container itself holds nothing: it is re-created at every start.
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
config=${KONUNG_CONFIG:-$repo/konung-server.conf}
# The host port: KONUNG_PORT, else the settings file's "port", else 7777.
port=${KONUNG_PORT:-}
if [[ -z $port && -f $config ]]; then
	port=$(sed -n 's/^[[:space:]]*port[[:space:]]*=[[:space:]]*"\{0,1\}\([0-9]*\)"\{0,1\}[[:space:]]*$/\1/p' "$config" | tail -n 1)
fi
port=${port:-7777}
assets=${KONUNG_ASSETS:-$repo/assets/Konung}
web=${KONUNG_WEB:-$repo/output/web}
go_image=${KONUNG_GO_IMAGE:-docker.io/library/golang:1.26}
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

# zl is the SELinux relabel option for bind mounts where SELinux enforces
# (Fedora); elsewhere none (Docker Desktop rejects unknown options).
zl=""
if [[ -r /sys/fs/selinux/enforce && $(cat /sys/fs/selinux/enforce) == 1 ]]; then
	zl=Z
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

# build_web builds the browser game (cmd/web-build) into $web inside a Go
# container, so the host needs no Go. The game files come from $assets; Go's
# downloads are kept in the volume konung-gocache for the next build. ffmpeg
# in the container repackages the converted films for the browser (without
# it the build still works, and the browser decodes them slowly).
build_web() {
	[[ $web == off ]] && return
	[[ -f $assets/GRAPH.RES ]] || die "$assets has no game files (copy the installed game there, see README.md)"
	say "Building the browser game into $web (a few minutes the first time) ..."
	mkdir -p "$web"
	# With rootful Docker the files would belong to root: give them back.
	local own=""
	if [[ $engine == docker ]]; then
		own=" && chown -R $(id -u):$(id -g) /out"
	fi
	"$engine" run --rm \
		-v "$repo:/src${zl:+:$zl}" -v "$assets:/assets:ro${zl:+,$zl}" -v "$web:/out${zl:+:$zl}" \
		-v konung-gocache:/go/pkg/mod -w /src \
		"$go_image" sh -c "apt-get update -qq && apt-get install -y -qq ffmpeg >/dev/null; go run ./cmd/web-build -assets /assets -out /out$own"
}

# copy_data puts GAME.RES (and SERVER, if present) into the volume through the
# created container.
copy_data() {
	[[ -f $assets/GAME.RES ]] || die "$assets/GAME.RES not found (set KONUNG_ASSETS); without it nobody can enter the world"
	"$engine" cp "$assets/GAME.RES" "$container:/data/GAME.RES"
	# The settings file, or an empty one so a removed file stops applying.
	if [[ -f $config ]]; then
		"$engine" cp "$config" "$container:/data/server.conf"
	else
		local empty
		empty=$(mktemp)
		"$engine" cp "$empty" "$container:/data/server.conf"
		rm -f "$empty"
	fi
	local f
	for f in SERVER konung.exe GAME.0 GAME.1 GAME.2 MAPS.RES GRAPH.RES OBJECTS.RES; do
		if [[ -f $assets/$f ]]; then
			"$engine" cp "$assets/$f" "$container:/data/$f"
		elif [[ $f != SERVER ]]; then
			say "Note: $assets/$f not found: fights between players or the server's world are off."
		fi
	done
	# The dialogues and trade (installed as questr.RES).
	if [[ -f $assets/questr.RES ]]; then
		"$engine" cp "$assets/questr.RES" "$container:/data/QUESTR.RES"
	elif [[ -f $assets/QUESTR.RES ]]; then
		"$engine" cp "$assets/QUESTR.RES" "$container:/data/QUESTR.RES"
	else
		say "Note: $assets/questr.RES not found: no dialogues or trade in the world."
	fi
}

create() {
	# Inside the container the server always listens on 7777 (mapped to the
	# host port); with the host's network it listens on the host port.
	local net=(-p "$port:7777/tcp" -p "$port:7777/udp")
	local args=(-port 7777)
	if [[ $network == host ]]; then
		net=(--network host)
		args=(-port "$port")
	fi
	if [[ -n ${KONUNG_NAME:-} ]]; then
		args+=(-name "$KONUNG_NAME")
	fi
	if [[ -n ${KONUNG_SERVER_ARGS:-} ]]; then
		# shellcheck disable=SC2206 # split like a command line on purpose
		eval "args+=($KONUNG_SERVER_ARGS)"
	fi
	# The browser game, read-only, served on the game port.
	# (Arrays are never empty here: macOS's bash 3.2 fails on empty ones
	# under set -u.)
	local mounts=(-v "$volume:/data")
	if [[ $web != off && -f $web/index.html ]]; then
		mounts+=(-v "$web:/web:ro${zl:+,$zl}")
		args+=(-web /web)
	fi
	"$engine" volume inspect "$volume" >/dev/null 2>&1 || "$engine" volume create "$volume" >/dev/null
	"$engine" create --name "$container" --restart unless-stopped \
		"${net[@]}" "${mounts[@]}" "$image" "${args[@]}" >/dev/null
}

# addresses prints how others reach this machine: its network addresses.
addresses() {
	local ips=""
	if command -v hostname >/dev/null && hostname -I >/dev/null 2>&1; then
		ips=$(hostname -I)
	elif command -v ipconfig >/dev/null; then # macOS
		ips=$(ipconfig getifaddr en0 2>/dev/null || true)
	fi
	local ip
	for ip in $ips; do
		[[ $ip == *:* ]] && continue # skip IPv6
		say "    http://$ip:$port/"
	done
}

start() {
	have_image || build
	if [[ $web != off && ! -f $web/index.html ]]; then
		build_web
	fi
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
	if [[ $web != off && -f $web/index.html ]]; then
		say "Browser game: on this machine http://localhost:$port/ ; for others on the network:"
		addresses
	fi
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
	build_web
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
web) build_web ;;
start) start ;;
stop) stop ;;
restart) stop; start ;;
update) update ;;
logs) "$engine" logs -f "$container" ;;
status) status ;;
reset) reset ;;
*)
	sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
