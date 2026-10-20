#!/bin/bash
set -euo pipefail

fail() {
    echo "macOS release: $*" >&2
    exit 1
}

cd "${GITHUB_WORKSPACE:-.}"
root=$(pwd -P)
tag=${RELEASE_TAG:-}
arch=${PACKAGE_ARCH:-}
pattern='^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,4})$'
[[ "$tag" =~ $pattern ]] || fail 'TAG must be vMAJOR.MINOR.PATCH without leading zeroes or suffixes.'
version=${tag#v}
IFS=. read -r major minor patch <<< "$version"
(( 10#$major <= 255 && 10#$minor <= 255 && 10#$patch <= 65535 )) || fail 'Version exceeds cross-platform limits (255.255.65535).'
[[ "$(uname -s)" == Darwin ]] || fail 'A native macOS host is required.'
case "$arch" in
    amd64) machine=x86_64 ;;
    arm64) machine=arm64 ;;
    *) fail 'ARCH must be amd64 or arm64.' ;;
esac
[[ "$(uname -m)" == "$machine" ]] || fail 'ARCH does not match the native macOS host.'
for tool in task go bun ditto plutil xattr codesign hdiutil lipo shasum git; do
    command -v "$tool" >/dev/null 2>&1 || fail "Missing tool: $tool"
done

stage=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/raven-macos.XXXXXX")
mountpoint="$stage/mounted"
mount_attempted=false
cleanup() {
    local status=$?
    trap - EXIT
    if [[ "$mount_attempted" == true ]]; then
        if ! hdiutil detach "$mountpoint"; then
            if mount | grep -Fq " on $mountpoint ("; then
                hdiutil detach -force "$mountpoint" || {
                    echo "Could not detach $mountpoint; preserving staging directory." >&2
                    exit 1
                }
            fi
        fi
    fi
    rm -rf -- "$stage"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Keep the release CLI isolated from any globally installed Wails version.
mkdir -p "$stage/tools" "$stage/image" "$mountpoint"
wails_version=$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)
[[ "$wails_version" == v* ]] || fail 'Could not determine the module-pinned Wails version.'
GOBIN="$stage/tools" go install "github.com/wailsapp/wails/v3/cmd/wails3@$wails_version"
export PATH="$stage/tools:$PATH"
task darwin:package "ARCH=$arch" DEV=false "BIN_DIR=$stage/build"

app="$stage/image/raven.app"
ditto "$stage/build/raven.app" "$app"
plist="$app/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$version" "$plist"
plutil -replace CFBundleVersion -string "$version" "$plist"
xattr -cr "$app"
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict "$app"
ln -s /Applications "$stage/image/Applications"

artifact="raven-$tag-macos-$arch"
dmg="$stage/$artifact.dmg"
hdiutil create -volname Raven -srcfolder "$stage/image" -format UDZO "$dmg"
hdiutil verify "$dmg"
mount_attempted=true
hdiutil attach -readonly -nobrowse -mountpoint "$mountpoint" "$dmg"

mounted_app="$mountpoint/raven.app"
mounted_plist="$mounted_app/Contents/Info.plist"
plutil -lint "$mounted_plist"
for key in CFBundleShortVersionString CFBundleVersion; do
    [[ "$(plutil -extract "$key" raw -o - "$mounted_plist")" == "$version" ]] || fail "Incorrect $key in DMG."
done
[[ "$(plutil -extract CFBundleExecutable raw -o - "$mounted_plist")" == raven ]] || fail 'Incorrect bundle executable.'
executable="$mounted_app/Contents/MacOS/raven"
[[ -x "$executable" ]] || fail 'Missing executable in DMG.'
[[ "$(lipo -archs "$executable")" == "$machine" ]] || fail 'Incorrect executable architecture in DMG.'
for resource in icons.icns raven.wav; do
    [[ -s "$mounted_app/Contents/Resources/$resource" ]] || fail "Missing resource: $resource"
done
if [[ -f "$app/Contents/Resources/Assets.car" ]]; then
    [[ -s "$mounted_app/Contents/Resources/Assets.car" ]] || fail 'Missing resource: Assets.car'
fi
[[ -L "$mountpoint/Applications" && "$(readlink "$mountpoint/Applications")" == /Applications ]] || fail 'Missing Applications shortcut.'
codesign --verify --deep --strict "$mounted_app"
hdiutil detach "$mountpoint"
mount_attempted=false

# A release must not depend on edits to generated bindings or other tracked files.
git diff --exit-code
git diff --cached --exit-code
(
    cd "$stage"
    shasum -a 256 "$artifact.dmg" > "$artifact.dmg.sha256"
)
mkdir -p "$root/bin"
cp "$dmg" "$dmg.sha256" "$root/bin/"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    {
        echo "artifact-name=$artifact"
        echo "dmg-path=bin/$artifact.dmg"
        echo "checksum-path=bin/$artifact.dmg.sha256"
    } >> "$GITHUB_OUTPUT"
fi
echo "Verified release: $artifact"
