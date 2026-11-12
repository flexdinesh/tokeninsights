#!/bin/sh
set -eu
# Builds the TokenInsights Native experiment from any ref on this machine.
# Only an explicit script path may select local helpers. Piped sh never trusts cwd.
case "$0" in
    */install-dev-native.sh)
        helper="$(dirname "$0")/install/source.sh"
        if [ -f "$0" ] && [ -f "$helper" ] && [ -f "$(dirname "$0")/install/lib.sh" ]; then exec sh "$helper" --product=desktop-native "$@"; fi
        ;;
esac
helpers="$(mktemp -d)"
trap 'rm -rf "$helpers"' EXIT INT TERM
for helper in source.sh lib.sh; do
    curl -fsSL --proto '=https' "https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install/$helper" -o "$helpers/$helper"
done
sh "$helpers/source.sh" --product=desktop-native "$@"
