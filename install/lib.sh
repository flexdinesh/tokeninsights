#!/bin/sh
# Common prebuilt product installer. No Go, Node, npm or pnpm required.
set -eu
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }
product=cli
channel=stable
for arg in "$@"; do
    case "$arg" in
        --product=cli|--product=server|--product=mygo|--product=electron) product="${arg#*=}" ;;
        --channel=stable|--channel=main) channel="${arg#*=}" ;;
        --help) printf '%s\n' 'Usage: install[-main|-server|-mygo|-electron].sh [--channel=stable|main]'; exit 0 ;;
        *) fail "unknown option $arg" ;;
    esac
done
for tool in curl tar awk mktemp; do need "$tool"; done
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) fail 'unsupported OS; use macOS or Linux' ;; esac
machine="$(uname -m)"
if [ "$os" = darwin ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then machine=arm64; fi
case "$machine" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) fail "unsupported architecture $machine" ;; esac
case "$channel" in
    stable) base=https://github.com/flexdinesh/tokeninsights/releases/latest/download ;;
    main) base=https://github.com/flexdinesh/tokeninsights/releases/download/main-build ;;
esac
base="${TOKENINSIGHTS_DOWNLOAD_URL:-$base}"
slug="tokeninsights-$product"
root="${slug}_${os}_${arch}"
archive="$root.tar.gz"
tmp="$(mktemp -d)"
stage=
backup=
dest=
cleanup() {
    if [ -n "$backup" ] && [ -d "$backup" ]; then
        rm -rf "$dest"
        mv "$backup" "$dest"
    fi
    if [ -n "$stage" ]; then rm -rf "$stage"; fi
    rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
download() { curl -fsSL --proto '=https,file' --retry 3 "$base/$1" -o "$tmp/$1" || fail "cannot download $1"; }
download "$archive"
download checksums.txt
expected="$(awk -v name="$archive" '$2 == name || $2 == "*" name { count++; hash=$1 } END { if (count != 1 || length(hash) != 64 || hash ~ /[^0-9a-f]/) exit 1; print hash }' "$tmp/checksums.txt")" || fail "checksums.txt requires one valid entry for $archive"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')"
else fail 'sha256sum or shasum is required'; fi
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive"
# Verify every member before extraction. Framework links stay relative/contained;
# no archive member may write through a symlink ancestor, regardless of ordering.
tar -tzf "$tmp/$archive" > "$tmp/entries" || fail 'invalid archive'
tar -tvzf "$tmp/$archive" > "$tmp/types" || fail 'invalid archive'
awk -v root="$root" '
function reject() { invalid=1; exit 1 }
function contained(path,    text,parts,stack,n,i,j,depth,prefix,remainder,steps,restart) {
    text=path
    for (steps=0; steps<=members; steps++) {
        n=split(text,parts,"/"); depth=0; prefix=""; restart=0
        for (i=1;i<=n;i++) {
            if (parts[i] == "" || parts[i] == ".") continue
            if (parts[i] == "..") {
                if (depth<=1) return 0
                depth--; prefix=stack[depth]; continue
            }
            prefix=(depth ? prefix "/" : "") parts[i]
            stack[++depth]=prefix
            if (depth==1 && prefix!=root) return 0
            if (prefix in links) {
                if (substr(links[prefix],1,1)=="/") return 0
                remainder=""
                for (j=i+1;j<=n;j++) remainder=remainder "/" parts[j]
                text=(depth>1 ? stack[depth-1] "/" : "") links[prefix] remainder
                restart=1; break
            }
        }
        if (!restart) return depth>=1
    }
    return 0
}
NR==FNR {
    path=$0; sub(/\/$/,"",path)
    if (path!=root && index(path,root "/")!=1) reject()
    if (path ~ /[[:cntrl:]\\]/ || index(path," -> ") || index(path," link to ")) reject()
    n=split(path,parts,"/")
    for (i=1;i<=n;i++) if (parts[i]==".." || parts[i]=="." || parts[i]=="") reject()
    if (path in names) reject()
    names[path]=1; members++; next
}
{
    type=substr($0,1,1)
    if (type!="-" && type!="d" && type!="l" && type!="h") reject()
    start=index($0,root "/"); if (!start) reject()
    path=substr($0,start)
    if (type=="l" || type=="h") {
        separator=(type=="l" ? " -> " : " link to ")
        splitAt=index(path,separator); if (!splitAt) reject()
        target=substr(path,splitAt+length(separator)); path=substr(path,1,splitAt-1)
        if (target=="" || target ~ /[[:cntrl:]\\]/) reject()
        if (type=="l") links[path]=target; else hardlinks[path]=target
    }
    sub(/\/$/,"",path)
    if (!(path in names) || path in kinds) reject()
    kinds[path]=type; listed++
}
END {
    if (invalid || members==0 || listed!=members) exit 1
    for (path in names) {
        n=split(path,parts,"/"); parent=parts[1]
        for (i=2;i<n;i++) { parent=parent "/" parts[i]; if (parent in links || parent in hardlinks) exit 1 }
    }
    for (path in links) if (!contained(path)) exit 1
    for (path in hardlinks) {
        target=hardlinks[path]
        if (!(target in kinds) || kinds[target]!="-" || !contained(target)) exit 1
    }
}' "$tmp/entries" "$tmp/types" || fail 'archive has unsafe members or links'
mkdir "$tmp/extracted"
tar -xzf "$tmp/$archive" -C "$tmp/extracted"
src="$(cd -P "$tmp/extracted/$root" && pwd)"
# Verify the actual filesystem too, including symlink chains and missing targets.
link_limit="$(wc -l < "$tmp/entries" | tr -d ' ')"
find "$src" -type l -exec sh -c '
    root=$1; limit=$2; shift 2
    for link do
        candidate=$link; steps=0
        while [ -L "$candidate" ]; do
            steps=$((steps+1)); [ "$steps" -le "$limit" ] || exit 1
            target=$(readlink "$candidate") || exit 1
            case "$target" in /*) exit 1 ;; esac
            candidate="$(dirname "$candidate")/$target"
            directory=$(cd -P "$(dirname "$candidate")" && pwd) || exit 1
            candidate="$directory/$(basename "$candidate")"
            if [ -d "$candidate" ]; then candidate=$(cd -P "$candidate" && pwd) || exit 1; fi
            case "$candidate" in "$root"|"$root"/*) ;; *) exit 1 ;; esac
        done
        [ -e "$candidate" ] || exit 1
    done
' sh "$src" "$link_limit" {} + || fail 'extracted links escape the product or have missing targets'
[ -f "$src/LICENSE" ] && [ -f "$src/build-info.json" ] || fail 'archive lacks license or build metadata'
[ -f "$src/product.txt" ] && [ "$(cat "$src/product.txt")" = "tokeninsights-product-v1 $product $os $arch" ] || fail 'incompatible product archive'
case "$product" in
    cli) binary=tokeninsights ;;
    server) binary=tokeninsights-server ;;
    mygo) binary=tokeninsights-mygo; app='TokenInsights MyGo.app'; identity=com.flexdinesh.tokeninsights.mygo ;;
    electron) binary=tokeninsights-electron; app='TokenInsights Electron.app'; identity=com.flexdinesh.tokeninsights.electron ;;
esac
if [ "$os" = darwin ] && { [ "$product" = mygo ] || [ "$product" = electron ]; }; then
    executable="$src/$app/Contents/MacOS/$binary"
    grep -F "<string>$identity</string>" "$src/$app/Contents/Info.plist" >/dev/null || fail 'incompatible app identity'
    backend="$src/$app/Contents/MacOS/tokeninsights-backend"
else
    executable="$src/$binary"
    backend="$src/tokeninsights-backend"
fi
if [ "$product" = electron ]; then
    if [ "$os" = darwin ]; then backend="$src/$app/Contents/Resources/tokeninsights-backend"; else backend="$src/resources/tokeninsights-backend"; fi
fi
[ -f "$executable" ] && [ -x "$executable" ] || fail "archive lacks executable $binary"
if [ "$product" != server ]; then
    [ -f "$backend" ] && [ -x "$backend" ] || fail 'archive lacks executable tokeninsights-backend'
fi
data="${XDG_DATA_HOME:-$HOME/.local/share}"
products="$data/tokeninsights/products"
dest="$products/$product"
marker="tokeninsights-product:$product"
if [ -e "$dest" ] || [ -L "$dest" ]; then
    [ ! -L "$dest" ] && [ -d "$dest" ] && [ -f "$dest/.installed-by-tokeninsights" ] &&
        [ "$(cat "$dest/.installed-by-tokeninsights")" = "$marker" ] || fail "refusing to replace unowned path $dest"
fi
bin="$HOME/.local/bin"
link="$bin/$binary"
target="$dest/$binary"
if [ "$os" = darwin ] && { [ "$product" = mygo ] || [ "$product" = electron ]; }; then
    bin="$HOME/Applications"
    link="$bin/$app"
    target="$dest/$app"
fi
if [ -e "$link" ] || [ -L "$link" ]; then
    [ -L "$link" ] && [ "$(readlink "$link")" = "$target" ] || fail "refusing to replace unowned path $link"
fi
if [ "$os" = linux ] && { [ "$product" = mygo ] || [ "$product" = electron ]; }; then
    entry="$data/applications/$slug.desktop"
    case "$dest" in *[\"\`\$\\%]*) fail 'install path cannot be represented in a desktop entry' ;; esac
    if [ -e "$entry" ] || [ -L "$entry" ]; then
        [ ! -L "$entry" ] && grep -Fx "X-TokenInsights-Product=$product" "$entry" >/dev/null || fail "refusing to replace unowned path $entry"
    fi
fi
mkdir -p "$products" "$bin"
stage="$(mktemp -d "$products/.$product.install-XXXXXX")"
cp -R "$src/." "$stage/"
printf '%s\n' "$marker" > "$stage/.installed-by-tokeninsights"
printf '%s\n' "$channel" > "$stage/.channel"
if [ -d "$dest" ]; then
    backup="$(mktemp -d "$products/.$product.previous-XXXXXX")"
    rmdir "$backup"
    mv "$dest" "$backup"
fi
mv "$stage" "$dest"
stage=
link_stage="$(mktemp -d "$bin/.tokeninsights-link-XXXXXX")"
ln -s "$target" "$link_stage/link"
rm -f "$link"
mv "$link_stage/link" "$link"
rmdir "$link_stage"
if [ "$os" = linux ] && { [ "$product" = mygo ] || [ "$product" = electron ]; }; then
    mkdir -p "$data/applications"
    cat > "$dest/desktop-entry" <<EOF
[Desktop Entry]
Type=Application
Name=TokenInsights $product
Exec="$dest/$binary" %u
Terminal=false
Categories=Utility;
MimeType=x-scheme-handler/tokeninsights-$product;
X-TokenInsights-Product=$product
EOF
    cp "$dest/desktop-entry" "$entry"
    if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$data/applications" >/dev/null 2>&1 || true; fi
    if command -v xdg-mime >/dev/null 2>&1; then
        mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}"
        xdg-mime default "$slug.desktop" "x-scheme-handler/tokeninsights-$product" || printf 'warning: open the app once to register links\n' >&2
    fi
fi
if [ "$os" = darwin ] && { [ "$product" = mygo ] || [ "$product" = electron ]; }; then
    register=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
    if [ -x "$register" ]; then "$register" -f "$dest/$app" || printf 'warning: open the app once to register links\n' >&2; fi
fi
if [ -n "$backup" ]; then rm -rf "$backup"; backup=; fi
printf 'Installed %s (%s): %s\n' "$product" "$channel" "$link"
