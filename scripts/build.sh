#!/usr/bin/env sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
cd "$project_root"
if command -v taskkill.exe >/dev/null 2>&1; then
	taskkill.exe //IM MorenoWoW.exe //F >/dev/null 2>&1 || true
elif command -v pkill >/dev/null 2>&1; then
	pkill -x MorenoWoW >/dev/null 2>&1 || pkill -x MorenoWoW.exe >/dev/null 2>&1 || true
fi
mkdir -p "$project_root/bin"
if [ "$(uname -s)" = "Linux" ] || [ "$(uname -s)" = "Darwin" ]; then
	go build -o "$project_root/bin/MorenoWoW" .
else
	go build -o "$project_root/bin/MorenoWoW.exe" .
fi
module_cache=$(go env GOMODCACHE)
if command -v cygpath >/dev/null 2>&1; then
	module_cache=$(cygpath -u "$module_cache")
fi
g3n_dll_path="$module_cache/github.com/g3n/engine@v0.2.0/audio/windows/bin"
if [ -d "$g3n_dll_path" ]; then
	cp "$g3n_dll_path"/*.dll "$project_root/bin/"
fi
