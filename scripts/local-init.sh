#!/bin/sh
# Panvara
# scripts/local-init.sh    2026-07-14
#
# @link    : https://github.com/shezw/panvara
# @author  : shezw
# @email   : hello@shezw.com

set -eu

if [ ! -f .env.example ]; then
  printf 'run make local-init from the Panvara repository root\n' >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  printf 'openssl is required to generate a local administrator token\n' >&2
  exit 1
fi

if [ ! -f .env ]; then
  cp .env.example .env
  printf '[created] .env from .env.example\n'
else
  printf '[kept] .env already exists\n'
fi

if [ ! -f .env.local ]; then
  umask 077
  token="$(openssl rand -hex 32)"
  printf 'PANVARA_ADMIN_TOKEN=%s\n' "$token" >.env.local
  chmod 600 .env.local
  printf '[created] .env.local with a private development token\n'
else
  printf '[kept] .env.local already exists; token was not replaced\n'
fi

printf '\nLocal configuration is ready. Load it in every new terminal with:\n'
printf '  set -a; . ./.env; . ./.env.local; set +a\n'
printf '\n.env and .env.local are ignored by Git. Do not commit them.\n'
