#!/usr/bin/env bash
# Taskfile-owned Linux packaging entry points. All generated files live in bin/.
set -euo pipefail

fail() { printf 'Linux packaging: %s\n' "$*" >&2; exit 1; }

require_tools() {
  local tool
  for tool in "$@"; do
    command -v "$tool" >/dev/null 2>&1 || fail "Missing tool: $tool (see task linux:install:deps)."
  done
}

validate_host() {
  [[ $(uname -s) == Linux ]] || fail 'A native Linux host is required.'
  local host
  case $(uname -m) in
    x86_64) host=amd64 ;;
    aarch64|arm64) host=arm64 ;;
    *) fail 'Only native amd64 and arm64 hosts are supported.' ;;
  esac
  [[ $arch == "$host" ]] || fail "ARCH=$arch does not match native host $host."
}

validate_version() {
  [[ $version =~ ^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,4})$ ]] ||
    fail 'VERSION must be MAJOR.MINOR.PATCH without leading zeroes or suffixes.'
  local major minor patch
  IFS=. read -r major minor patch <<< "$version"
  (( 10#$major <= 255 && 10#$minor <= 255 && 10#$patch <= 65535 )) ||
    fail 'Version exceeds cross-platform limits (255.255.65535).'
  [[ ${RAVEN_LINUX_DEV:-false} == false ]] || fail 'Packaging requires DEV=false.'
  [[ -z $tag || $tag == "v$version" ]] || fail 'TAG must equal vVERSION.'
}

validate_build() {
  validate_host
  require_tools go bun wails3 cc pkg-config
  [[ $(go env GOVERSION) == "go$(awk '$1 == "go" {print $2}' go.mod)" ]] ||
    fail 'Go must match go.mod.'
  [[ $(bun --version) == "$(bun -p "require('./package.json').packageManager.slice(4)")" ]] ||
    fail 'Bun must match package.json.'
  pkg-config --exists gtk4 webkitgtk-6.0 gio-unix-2.0 || fail 'GTK4/WebKitGTK 6.0 development libraries are required.'
  if [[ ${RAVEN_LINUX_OBFUSCATED:-false} == true ]]; then require_tools garble; fi
}

validate_package() {
  validate_host
  validate_version
  case $format in
    all) formats=(deb rpm pacman) ;;
    deb|rpm|pacman) formats=("$format") ;;
    *) fail 'FORMAT must be all, deb, rpm, or pacman.' ;;
  esac
}

clean_sources() {
  if ! git diff --exit-code --quiet || ! git diff --cached --exit-code --quiet; then
    fail 'Release packaging requires unchanged tracked sources.'
  fi
}

validate_release() {
  validate_package
  [[ $tag == "v$version" ]] || fail 'TAG is required: vMAJOR.MINOR.PATCH.'
  [[ $format == all ]] || fail 'Releases require all three formats.'
  clean_sources
}

install_deps() {
  validate_host
  # shellcheck source=/dev/null
  source /etc/os-release
  [[ $ID == ubuntu && $VERSION_ID == 26.04 ]] ||
    fail 'Automatic dependency setup requires Ubuntu 26.04.'
  local elevate=()
  if (( EUID != 0 )); then elevate=(sudo); fi
  "${elevate[@]}" apt-get update
  "${elevate[@]}" apt-get install -y --no-install-recommends build-essential pkg-config \
    libgtk-4-dev libwebkitgtk-6.0-dev libnotify-dev dpkg-dev desktop-file-utils \
    file binutils rpm libarchive-tools zstd \
    xvfb xauth xdotool dbus-x11

}

install_tooling() {
  validate_host
  require_tools go bun
  local go_version wails_version
  go_version=$(awk '$1 == "go" {print $2}' go.mod)
  export GOTOOLCHAIN="go$go_version"
  wails_version=$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)
  [[ $wails_version == v* ]] || fail 'Could not determine the pinned Wails version.'
  GOBIN="$root/.tools" go install "github.com/wailsapp/wails/v3/cmd/wails3@$wails_version"
}

frontend() {
  # A nested Task invocation inherits this PATH, including the pinned local Wails CLI.
  # Include-level env does not carry across to common's binding-generation tasks.
  require_tools task
  task common:build:frontend "BUILD_FLAGS=${RAVEN_LINUX_BUILD_FLAGS:-}" \
    "OBFUSCATED=${RAVEN_LINUX_OBFUSCATED:-false}" "DEV=${RAVEN_LINUX_DEV:-false}"
}

desktop() {
  mkdir -p "$payload/usr/share/applications" "$payload/usr/share/icons/hicolor/1024x1024/apps"
  cp "$root/build/linux/raven.desktop" "$payload/usr/share/applications/raven.desktop"
  cp "$root/build/appicon.png" "$payload/usr/share/icons/hicolor/1024x1024/apps/raven.png"
}

extension() {
  case $1 in pacman) printf pkg.tar.zst ;; *) printf '%s' "$1" ;; esac
}

prepare() {
  validate_package
  validate_build
  require_tools readelf sha256sum desktop-file-validate dbus-run-session xvfb-run xdotool timeout
  local kind
  for kind in "${formats[@]}"; do
    case $kind in
      deb) require_tools dpkg-deb ;;
      rpm) require_tools rpm bsdtar ;;
      pacman) require_tools tar zstd ;;
    esac
  done
  rm -rf "$stage"
  mkdir -p "$payload/usr/bin" "$stage/packages" "$stage/verified"
  # Remove this candidate's old promoted outputs so a failed rerun cannot leave a stale release.
  for kind in "${formats[@]}"; do
    rm -f "$root/bin/$artifact.$(extension "$kind")" "$root/bin/$artifact.$(extension "$kind").sha256"
  done
  desktop
  desktop-file-validate "$payload/usr/share/applications/raven.desktop"
  printf '%s\n' "$version|$format|$tag" > "$stage/prepared"
}

require_stage() {
  [[ -f $stage/prepared && $(cat "$stage/prepared") == "$version|$format|$tag" ]] ||
    fail 'Staging does not match this candidate; rerun linux:package.'
  [[ -x $payload/usr/bin/raven ]] || fail 'Missing staged Raven executable.'
}

bundle() {
  validate_package
  require_stage
  export RAVEN_LINUX_ARCH="$arch" RAVEN_LINUX_VERSION="$version" RAVEN_LINUX_PAYLOAD="$payload"
  local kind output
  for kind in "${formats[@]}"; do
    output="$stage/packages/$artifact.$(extension "$kind")"
    case $kind in
      deb|rpm|pacman)
        local packager=$kind
        [[ $kind != pacman ]] || packager=archlinux
        wails3 tool package -name "$artifact" -format "$packager" \
          -config "$root/build/linux/nfpm/nfpm.yaml" -out "$stage/packages"
        ;;
    esac
    [[ -s $output ]] || fail "Missing package: $output"
  done
}

expect() { [[ $1 == "$2" ]] || fail "Expected $2; found $1 ($3)."; }

verify_elf() {
  [[ -x $1 ]] || fail "Missing executable: $1"
  readelf -h "$1" | grep -Eq "Machine:.*$elf_machine" || fail "Incorrect ELF architecture: $1"
  cmp "$payload/usr/bin/raven" "$1" || fail "Package executable differs from the built payload: $1"
}

verify_payload() {
  local directory=$1
  verify_elf "$directory/usr/bin/raven"
  cmp "$payload/usr/share/applications/raven.desktop" "$directory/usr/share/applications/raven.desktop"
  cmp "$payload/usr/share/icons/hicolor/1024x1024/apps/raven.png" \
    "$directory/usr/share/icons/hicolor/1024x1024/apps/raven.png"
  desktop-file-validate "$directory/usr/share/applications/raven.desktop"
}

verify() {
  validate_package
  require_stage
  local kind file directory metadata
  for kind in "${formats[@]}"; do
    file="$stage/packages/$artifact.$(extension "$kind")"
    [[ -s $file && ! -L $file ]] || fail "Missing regular package: $file"
    directory="$stage/verified/$kind"
    rm -rf "$directory"
    mkdir -p "$directory"
    case $kind in
      deb)
        expect "$(dpkg-deb -f "$file" Package)" raven 'DEB name'
        expect "$(dpkg-deb -f "$file" Version)" "$version-1" 'DEB version'
        expect "$(dpkg-deb -f "$file" Architecture)" "$arch" 'DEB architecture'
        metadata=$(dpkg-deb -f "$file" Depends)
        [[ $metadata == *libgtk-4-1* && $metadata == *libwebkitgtk-6.0-4* ]] || fail 'Missing DEB runtime dependencies.'
        dpkg-deb -x "$file" "$directory"
        verify_payload "$directory"
        ;;
      rpm)
        expect "$(rpm -qp --qf '%{NAME}' "$file")" raven 'RPM name'
        expect "$(rpm -qp --qf '%{VERSION}-%{RELEASE}' "$file")" "$version-1" 'RPM version'
        expect "$(rpm -qp --qf '%{ARCH}' "$file")" "$native_arch" 'RPM architecture'
        metadata=$(rpm -qp --requires "$file")
        if ! grep -qx gtk4 <<< "$metadata" || ! grep -qx webkitgtk6.0 <<< "$metadata"; then
          fail 'Missing RPM runtime dependencies.'
        fi
        # libarchive strips absolute member paths and avoids rpm2cpio's nonzero
        # producer status on otherwise complete nFPM payloads.
        bsdtar -xf "$file" -C "$directory" --no-same-owner
        verify_payload "$directory"
        ;;
      pacman)
        tar --zstd -xf "$file" -C "$directory"
        metadata="$directory/.PKGINFO"
        grep -qx 'pkgname = raven' "$metadata"
        grep -qx "pkgver = $version-1" "$metadata"
        grep -qx "arch = $native_arch" "$metadata"
        grep -qx 'depend = gtk4' "$metadata"
        grep -qx 'depend = webkitgtk-6.0' "$metadata"
        verify_payload "$directory"
        ;;
    esac
  done
}

smoke_session() {
  "$@" &
  local pid=$! attempt
  # Capture the numeric PID now; the EXIT trap runs after this function returns.
  # shellcheck disable=SC2064
  trap "kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true" EXIT
  for ((attempt = 0; attempt < 25; attempt++)); do
    kill -0 "$pid" 2>/dev/null || fail 'Application exited during startup.'
    if xdotool search --onlyvisible --name 'Raven' >/dev/null 2>&1; then
      sleep 5
      kill -0 "$pid" 2>/dev/null || fail 'Application exited after showing its window.'
      return
    fi
    sleep 1
  done
  fail 'No visible Raven window appeared during startup.'
}

smoke_command() {
  local label=$1
  shift
  local status=0 log="$stage/$label.log"
  # Require a visible native window and continued liveness; retain WebKit's sandbox.
  timeout --signal=TERM --kill-after=5s 40s dbus-run-session -- \
    xvfb-run -a --server-args='-screen 0 1280x800x24 -nolisten tcp' \
    env GDK_BACKEND=x11 GSK_RENDERER=cairo LIBGL_ALWAYS_SOFTWARE=1 \
    XDG_CONFIG_HOME="$stage/config-$label" XDG_DATA_HOME="$stage/data-$label" \
    bash "$root/taskfiles/linux/funcs.sh" smoke-session "$@" > "$log" 2>&1 || status=$?
  if [[ $status != 0 ]]; then cat "$log" >&2; fail "$label GUI startup check failed (status $status)."; fi
  if grep -Eiq 'panic:|error while loading shared libraries|cannot open display|Failed to execute child process|bwrap:.*(denied|not permitted)' "$log"; then
    cat "$log" >&2
    fail "$label reported a native startup error."
  fi
}

smoke() {
  validate_package
  require_stage
  local kind directory
  for kind in "${formats[@]}"; do
    directory="$stage/verified/$kind"
    smoke_command "$kind" "$directory/usr/bin/raven"
  done
  (cd "$stage/packages" && sha256sum ./*) > "$stage/smoked.sha256"
}

promote() {
  validate_package
  require_stage
  [[ -f $stage/smoked.sha256 ]] || fail 'Packages must pass GUI startup checks before promotion.'
  (cd "$stage/packages" && sha256sum --check "$stage/smoked.sha256")
  local kind name
  for kind in "${formats[@]}"; do
    name="$artifact.$(extension "$kind")"
    (cd "$stage/packages" && sha256sum "$name") > "$stage/packages/$name.sha256"
  done
  # All formats have passed before any public output is copied.
  for kind in "${formats[@]}"; do
    name="$artifact.$(extension "$kind")"
    cp "$stage/packages/$name" "$stage/packages/$name.sha256" "$root/bin/"
  done
}

release_outputs() {
  validate_release
  local kind name
  for kind in "${formats[@]}"; do
    name="$artifact.$(extension "$kind")"
    [[ -s $root/bin/$name && ! -L $root/bin/$name ]] || fail "Missing release output: $name"
    (cd "$root/bin" && sha256sum --check "$name.sha256")
  done
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    {
      printf 'artifact-name=%s\n' "$artifact"
      printf 'package-paths<<RAVEN_LINUX_PACKAGES\n'
      for kind in "${formats[@]}"; do printf 'bin/%s.%s\n' "$artifact" "$(extension "$kind")"; done
      printf 'RAVEN_LINUX_PACKAGES\nchecksum-paths<<RAVEN_LINUX_CHECKSUMS\n'
      for kind in "${formats[@]}"; do printf 'bin/%s.%s.sha256\n' "$artifact" "$(extension "$kind")"; done
      printf 'RAVEN_LINUX_CHECKSUMS\n'
    } >> "$GITHUB_OUTPUT"
  fi
}

main() {
  local command=${1:-}
  root=${RAVEN_LINUX_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
  cd "$root"
  root=$PWD
  arch=${RAVEN_LINUX_ARCH:-}
  case $arch in
    amd64) native_arch=x86_64; elf_machine='Advanced Micro Devices X86-64' ;;
    arm64) native_arch=aarch64; elf_machine=AArch64 ;;
    *) fail 'ARCH must be amd64 or arm64.' ;;
  esac
  version=${RAVEN_LINUX_VERSION:-}
  tag=${RAVEN_LINUX_TAG:-}
  format=${RAVEN_LINUX_FORMAT:-all}
  stage="$root/bin/.linux/$arch"
  payload="$stage/payload"
  artifact="raven-v$version-linux-$arch"
  export PATH="$root/.tools:$PATH"
  case $command in
    validate-host) validate_host ;;
    validate-build) validate_build ;;
    validate-package) validate_package ;;
    validate-release) validate_release ;;
    install-deps) install_deps ;;
    install-tooling) install_tooling ;;
    frontend) frontend ;;
    desktop) validate_host; desktop ;;
    prepare) prepare ;;
    bundle) bundle ;;
    verify) verify ;;
    smoke) smoke ;;
    smoke-session) shift; smoke_session "$@" ;;
    promote) promote ;;
    release-outputs) release_outputs ;;
    *) fail "Unknown command: $command" ;;
  esac
}

main "$@"
