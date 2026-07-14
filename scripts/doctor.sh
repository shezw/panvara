#!/bin/sh
# Panvara
# scripts/doctor.sh    2026-07-14
#
# @link    : https://github.com/shezw/panvara
# @author  : shezw
# @email   : hello@shezw.com

set -u

mode="${1:-lite}"
failures=0

ok() {
  printf '[ok] %s\n' "$1"
}

fail() {
  printf '[missing] %s\n' "$1" >&2
  failures=$((failures + 1))
}

check_command() {
  label="$1"
  command_name="$2"
  if command -v "$command_name" >/dev/null 2>&1; then
    ok "$label: $(command -v "$command_name")"
  else
    fail "$label: command '$command_name' was not found"
  fi
}

printf 'Panvara local environment check (%s)\n' "$mode"

if [ ! -f go.mod ] || [ ! -f .env.example ]; then
  fail "run this command from the Panvara repository root"
fi

check_command "Git" git
check_command "Go" go
check_command "Make" make
check_command "curl" curl

if command -v go >/dev/null 2>&1; then
  go_version="$(GOTOOLCHAIN=local go version 2>&1)"
  case "$go_version" in
    *"go1."*)
      go_minor="$(printf '%s' "$go_version" | sed -n 's/.*go1\.\([0-9][0-9]*\).*/\1/p')"
      if [ -n "$go_minor" ] && [ "$go_minor" -ge 25 ]; then
        ok "$go_version"
      else
        fail "$go_version; Panvara requires Go 1.25 or newer and recommends Go 1.26.5"
      fi
      ;;
    *) fail "could not read the installed Go version: $go_version" ;;
  esac
fi

if [ "$mode" = "server" ]; then
  check_command "Docker" docker
  check_command "OpenSSL" openssl
  if command -v docker >/dev/null 2>&1; then
    if docker compose version >/dev/null 2>&1; then
      ok "Docker Compose plugin"
    else
      fail "Docker Compose plugin is unavailable"
    fi
    if docker compose up --help 2>/dev/null | grep -q -- '--wait'; then
      ok "Docker Compose supports 'up --wait'"
    else
      fail "Docker Compose is too old for 'up --wait'; upgrade Docker Compose"
    fi
    if docker info >/dev/null 2>&1; then
      ok "Docker daemon is running"
    else
      fail "Docker daemon is not reachable; start Docker Desktop or Docker Engine"
    fi
  fi
elif [ "$mode" != "lite" ]; then
  fail "unknown mode '$mode'; select lite or server"
fi

if [ "$failures" -ne 0 ]; then
  printf '\nEnvironment check failed with %s problem(s).\n' "$failures" >&2
  exit 1
fi

printf '\nEnvironment check passed.\n'
