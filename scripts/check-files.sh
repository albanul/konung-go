#!/usr/bin/env bash
# Check that the game files are where the port looks for them, and say what
# is missing and what that costs.
#
#   scripts/check-files.sh                 checks assets/Konung
#   scripts/check-files.sh /path/to/Konung checks another folder
set -uo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
dir=${1:-$repo/assets/Konung}
missing_required=0

say() { printf '%s\n' "$*"; }

# has finds a file in $dir whatever the case of its name (the installed
# names differ: questr.RES, Intr800r.res).
has() { [[ -n $(find "$dir" -maxdepth 1 -iname "$1" -print -quit 2>/dev/null) ]]; }

# group checks files that one feature needs: "required" ones stop the game.
group() {
	local kind=$1 what=$2
	shift 2
	local missing=()
	local f
	for f in "$@"; do
		has "$f" || missing+=("$f")
	done
	if [[ ${#missing[@]} -eq 0 ]]; then
		say "  OK       $what"
	elif [[ $kind == required ]]; then
		say "  MISSING  $what: ${missing[*]}"
		missing_required=1
	else
		say "  missing  $what (optional): ${missing[*]}"
	fi
}

if [[ ! -d $dir ]]; then
	say "No folder $dir."
	say "Copy the installed game there (see README.md, «Step 1: the game files»)."
	exit 1
fi
say "Game files in $dir:"
group required "the game" konung.exe GRAPH.RES OBJECTS.RES MENU800R.RES NEWH800R.RES SAVE800R.RES \
	Intr800r.res KONUNG.1 GAME.0 GAME.1 GAME.2
group optional "dialogues and trade (without it nobody talks)" QUESTR.RES
group optional "glows and night lights" LIGHTS.RES LIGHTS.1
group optional "sound and music" SOUNDS.RES MUSICS.RES
group optional "fire" FLAMES.RES
group optional "the network game «Сетевая игра» (hidden without these)" LAN_KONG.EXE ME800R.RES PLAY800R.RES NEW800R.RES CLANS.CFG
group optional "a network game server (scripts/konung-server.sh)" GAME.RES MAPS.RES INTO640R.RES
films=$(find "$dir/Video" -maxdepth 1 -iname '*.flm' 2>/dev/null | wc -l | tr -d ' ')
converted=$(find "$dir/Video" -maxdepth 1 -iname '*.ivf' 2>/dev/null | wc -l | tr -d ' ')
if [[ $films -eq 0 ]]; then
	say "  missing  the films (optional): no Video/*.flm; the game skips them"
elif [[ $converted -lt $films ]]; then
	say "  note     the films are not converted yet ($converted of $films): the game skips them (see README.md, «Films»)"
else
	say "  OK       the films ($converted converted)"
fi
if [[ $missing_required -ne 0 ]]; then
	say "Some required files are missing: the game will not start."
	exit 1
fi
say "All set."
