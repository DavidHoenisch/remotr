#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdocs build

if [ -d demo/assets ] && compgen -G "demo/assets/*" >/dev/null; then
  mkdir -p site/assets/demo
  cp -a demo/assets/. site/assets/demo/
fi

cp -r hub site/hub
