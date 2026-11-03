#!/bin/sh
set -eu
# Only an explicit script path may select a local helper. Piped sh never trusts cwd.
case "$0" in
    */install-server.sh)
        helper="$(dirname "$0")/install/lib.sh"
        if [ -f "$0" ] && [ -f "$helper" ]; then exec sh "$helper" --product=server --channel=stable "$@"; fi
        ;;
esac
helper="$(mktemp)"
trap 'rm -f "$helper"' EXIT INT TERM
curl -fsSL --proto '=https' https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install/lib.sh -o "$helper"
sh "$helper" --product=server --channel=stable "$@"
