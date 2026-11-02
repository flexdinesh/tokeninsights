#!/bin/sh
# Installs the TokenInsights desktop viewer for the current user (macOS, Linux).
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh
#
# The app is a thin viewer: `tokeninsights web` opens its dashboard in it.
# Install the CLI separately (Homebrew or go install). Rerun to upgrade.
# TOKENINSIGHTS_DOWNLOAD_URL overrides the release download base URL.
set -eu

download_url="${TOKENINSIGHTS_DOWNLOAD_URL:-https://github.com/flexdinesh/tokeninsights/releases/latest/download}"
bundle_id="com.flexdinesh.tokeninsights"
slug="tokeninsights-desktop"
scheme="tokeninsights"

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }

detect_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) fail "unsupported OS $(uname -s); the desktop app supports macOS and Linux" ;;
	esac
}

detect_arch() {
	machine="$(uname -m)"
	# Rosetta reports x86_64 on Apple silicon; install the native app.
	if [ "$1" = darwin ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
		machine=arm64
	fi
	case "$machine" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture $machine; the desktop app supports amd64 and arm64" ;;
	esac
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		fail "sha256sum or shasum is required"
	fi
}

download() {
	curl -fsSL --proto '=https,file' --retry 3 -o "$2" "$download_url/$1" || fail "cannot download $download_url/$1"
}

verify() {
	expected="$(awk -v name="$2" '$2 == name || $2 == "*" name { print $1 }' "$3")"
	[ -n "$expected" ] || fail "checksums.txt has no entry for $2"
	[ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" = 1 ] || fail "checksums.txt has duplicate entries for $2"
	actual="$(sha256 "$1")"
	[ "$actual" = "$expected" ] || fail "checksum mismatch for $2"
}

install_macos() {
	app="$1/TokenInsights.app"
	[ -d "$app" ] || fail "archive has no TokenInsights.app"
	apps="$HOME/Applications"
	dest="$apps/TokenInsights.app"
	mkdir -p "$apps"
	if [ -e "$dest" ]; then
		existing="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$dest/Contents/Info.plist" 2>/dev/null || true)"
		[ "$existing" = "$bundle_id" ] || fail "$dest exists and is not TokenInsights; move it away and rerun"
	fi
	staged="$apps/.TokenInsights.app.installing"
	rm -rf "$staged"
	cp -R "$app" "$staged"
	rm -rf "$dest"
	mv "$staged" "$dest"
	lsregister=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
	if [ -x "$lsregister" ]; then
		"$lsregister" -f "$dest" || warn "cannot register $scheme:// links; open the app once to register them"
	fi
	say "Installed $dest"
}

desktop_exec_path() {
	case "$1" in
	*[\"\`\$\\]*) fail "install path $1 contains characters a desktop entry cannot quote" ;;
	esac
}

has_webkit() {
	if command -v ldconfig >/dev/null 2>&1 && ldconfig -p 2>/dev/null | grep -q 'libwebkit2gtk-4\.[01]\.so'; then
		return 0
	fi
	for dir in /usr/lib /usr/lib64 /usr/lib/x86_64-linux-gnu /usr/lib/aarch64-linux-gnu /usr/local/lib; do
		for lib in "$dir"/libwebkit2gtk-4.1.so* "$dir"/libwebkit2gtk-4.0.so*; do
			[ -e "$lib" ] && return 0
		done
	done
	return 1
}

install_linux() {
	src="$1"
	[ -f "$src/$slug" ] || fail "archive has no $slug executable"
	data="${XDG_DATA_HOME:-$HOME/.local/share}"
	dest="$data/$slug"
	entries="$data/applications"
	desktop_exec_path "$dest/$slug"
	mkdir -p "$dest" "$entries"
	staged="$dest/.$slug.installing"
	cp "$src/$slug" "$staged"
	chmod 755 "$staged"
	mv "$staged" "$dest/$slug"
	cp "$src/$slug.png" "$dest/$slug.png"
	entry="$entries/$slug.desktop"
	cat >"$entry.installing" <<EOF
[Desktop Entry]
Type=Application
Name=TokenInsights
Comment=TokenInsights dashboard viewer
Exec="$dest/$slug" %u
Icon=$dest/$slug.png
Categories=Utility;
Terminal=false
MimeType=x-scheme-handler/$scheme;
EOF
	mv "$entry.installing" "$entry"
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$entries" >/dev/null 2>&1 || warn "update-desktop-database failed"
	fi
	if command -v xdg-mime >/dev/null 2>&1; then
		# xdg-mime writes mimeapps.list there and fails on a fresh home without it.
		mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}"
		xdg-mime default "$slug.desktop" "x-scheme-handler/$scheme" || warn "cannot register $scheme:// links with xdg-mime"
		[ "$(xdg-mime query default "x-scheme-handler/$scheme" 2>/dev/null)" = "$slug.desktop" ] ||
			warn "$scheme:// links are not registered; tokeninsights web will open the browser"
	else
		warn "xdg-mime not found; install xdg-utils so tokeninsights web can open the app"
	fi
	say "Installed $dest/$slug"
	if ! has_webkit; then
		warn "WebKitGTK 4.1 not found; the app needs it to run. Install it with one of:"
		warn "  sudo apt install libwebkit2gtk-4.1-0    (Debian/Ubuntu)"
		warn "  sudo dnf install webkit2gtk4.1          (Fedora)"
		warn "  sudo pacman -S webkit2gtk-4.1           (Arch)"
		warn "Until then, tokeninsights web opens the browser."
	fi
}

main() {
	need curl
	need tar
	need awk
	os="$(detect_os)"
	arch="$(detect_arch "$os")"
	archive="${slug}_${os}_${arch}.tar.gz"
	tmp="$(mktemp -d 2>/dev/null || mktemp -d -t tokeninsights)"
	trap 'rm -rf "$tmp"' EXIT INT TERM
	say "Downloading $archive"
	download "$archive" "$tmp/$archive"
	download checksums.txt "$tmp/checksums.txt"
	verify "$tmp/$archive" "$archive" "$tmp/checksums.txt"
	tar -xzf "$tmp/$archive" -C "$tmp"
	root="$tmp/${slug}_${os}_${arch}"
	[ -d "$root" ] || fail "archive has an unexpected layout"
	case "$os" in
	darwin) install_macos "$root" ;;
	linux) install_linux "$root" ;;
	esac
	say "Run tokeninsights web to open your dashboard in the app."
	say "Use tokeninsights web --open=browser to keep using the browser."
}

main "$@"
