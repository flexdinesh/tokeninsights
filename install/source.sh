#!/bin/sh
# Builds one product from a canonical ref on this machine, then installs it with lib.sh.
# Requires git and mise; mise installs the locked Go, Node and pnpm toolchain.
set -eu
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }
product=
ref=main
cli_only=false
modify_path=true
for arg in "$@"; do
    case "$arg" in
        --product=desktop|--product=desktop-native) product="${arg#*=}" ;;
        --ref=*) ref="${arg#*=}" ;;
        --cli-only) cli_only=true ;;
        --no-modify-path) modify_path=false ;;
        --help)
            if [ "$product" = desktop-native ]; then
                printf '%s\n' 'Usage: install-dev-native.sh [--ref=<branch|tag|sha>]'
            else
                printf '%s\n' 'Usage: install-dev.sh [--ref=<branch|tag|sha>] [--cli-only] [--no-modify-path]'
            fi
            printf '%s\n' 'Builds the ref (default main) locally. Requires git and mise.'
            exit 0
            ;;
        *) fail "unknown option $arg" ;;
    esac
done
[ -n "$product" ] || fail 'source build requires --product'
if [ "$cli_only" = true ]; then
    [ "$product" = desktop ] || fail 'TokenInsights Native has no CLI-only build; omit --cli-only'
    product=cli
fi
case "$ref" in ''|-*|*[[:space:]]*) fail "invalid ref '$ref'" ;; esac
for tool in git curl tar mktemp; do need "$tool"; done
command -v mise >/dev/null 2>&1 || fail 'mise is required to build from source; install it from https://mise.jdx.dev and rerun'
git check-ref-format "refs/heads/$ref" || fail "invalid ref '$ref'"
lib="$(cd "$(dirname "$0")" && pwd)/lib.sh"
[ -f "$lib" ] || fail "missing installer helper $lib"
repo="${TOKENINSIGHTS_SOURCE_URL:-https://github.com/flexdinesh/tokeninsights.git}"
# Resolve once; every later step uses this exact commit. A branch wins over a same-named tag.
if printf '%s\n' "$ref" | grep -Eq '^[0-9a-f]{40}$'; then
    commit="$ref"
else
    listing="$(git ls-remote "$repo" "refs/heads/$ref" "refs/tags/$ref" "refs/tags/$ref^{}")" || fail "cannot reach $repo"
    commit="$(printf '%s\n' "$listing" | awk -v head="refs/heads/$ref" -v tag="refs/tags/$ref" '
        $2 == head { branch = $1 } $2 == tag "^{}" { peeled = $1 } $2 == tag { annotated = $1 }
        END { if (branch != "") print branch; else if (peeled != "") print peeled; else if (annotated != "") print annotated }')"
    [ -n "$commit" ] || fail "unknown ref '$ref' in $repo"
fi
short="$(printf '%s' "$commit" | cut -c1-12)"
cache="${XDG_CACHE_HOME:-$HOME/.cache}/tokeninsights/source"
lock="$cache.lock"
mkdir -p "$(dirname "$cache")"
mkdir "$lock" 2>/dev/null || fail "another source install holds $lock; remove it if none is running"
tmp=
cleanup() {
    if [ -n "$tmp" ]; then rm -rf "$tmp"; fi
    rmdir "$lock"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
tmp="$(mktemp -d)"
if [ -e "$cache" ] && [ ! -d "$cache/.git" ]; then fail "refusing to use $cache: not a source cache"; fi
[ -d "$cache/.git" ] || git init -q "$cache"
printf 'Building %s from %s (%s)\n' "$product" "$ref" "$short"
git -C "$cache" fetch -q --depth=1 --no-tags "$repo" "$commit" || fail "cannot fetch $commit from $repo"
git -C "$cache" -c advice.detachedHead=false checkout -q --force --detach "$commit"
# Dependencies survive between builds; every other untracked or ignored file is rebuilt.
git -C "$cache" clean -q -ffdx -e node_modules
[ "$(git -C "$cache" rev-parse HEAD)" = "$commit" ] || fail "checkout does not match $commit"
[ -f "$cache/install/source.sh" ] || fail "ref '$ref' predates source install support"
cd "$cache"
# Build tooling runs from the checkout; no Git hooks are installed into the cache.
export HUSKY=0
mise trust -q "$cache/mise.toml"
mise install
mise exec -- pnpm install --frozen-lockfile
mise run build:web
mise exec -- node tools/build/src/build-products.ts "src.$short" "$tmp/dist" "$product" "--ref=$ref"
set -- --channel=source "--product=$product"
if [ "$modify_path" = false ]; then set -- "$@" --no-modify-path; fi
TOKENINSIGHTS_DOWNLOAD_URL="file://$tmp/dist" sh "$lib" "$@"
