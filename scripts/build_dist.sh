#!/usr/bin/env bash
# Build cross-platform distribution folders for Kasa Light Control backend + frontend.
# This script compiles the Go server for several target operating systems and
# assembles self-contained distribution directories under ./dist.
#
# Usage: ./scripts/build_dist.sh
# Requires Go toolchain installed and accessible in PATH.

set -euo pipefail

APP_NAME="kasaserver"           # Binary name for server
STATIC_DIR="cmd/kasaserver/static" # Frontend files to ship with server
DIST_DIR="dist"                  # Root output directory

# Clean previous builds
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

build_target() {
  local goos=$1
  local goarch=$2
  local folder=$3
  local ext=$4

  echo "\n==> Building ${folder} (GOOS=${goos} GOARCH=${goarch})"
  local out_dir="${DIST_DIR}/${folder}"
  mkdir -p "$out_dir"

  # Compile backend
  env GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -o "${out_dir}/${APP_NAME}${ext}" ./cmd/kasaserver

  # Copy frontend assets
  cp -r "$STATIC_DIR" "${out_dir}/static"

  # Optionally include README for ease of use
  if [[ -f README.md ]]; then
    cp README.md "$out_dir/README.md"
  fi
}

# Windows 11 / amd64
build_target windows amd64 windows-amd64 .exe

# Generic Linux x86_64
build_target linux amd64 linux-amd64 ""

# macOS (darwin) on Apple Silicon
build_target darwin arm64 darwin-arm64 ""

# macOS .app packaging function
package_macos_app() {
  local bundle_dir="${DIST_DIR}/KasaLightControl.app/Contents"
  echo "\n==> Creating macOS .app bundle at ${bundle_dir}"
  mkdir -p "${bundle_dir}/MacOS" "${bundle_dir}/Resources"

  # Copy binary
  cp "${DIST_DIR}/darwin-arm64/${APP_NAME}" "${bundle_dir}/MacOS/${APP_NAME}"
  chmod +x "${bundle_dir}/MacOS/${APP_NAME}"

  # Copy frontend assets
  cp -R "${STATIC_DIR}" "${bundle_dir}/Resources/"

  # Generate minimal Info.plist
  cat > "${bundle_dir}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>         <string>Kasa Light Control</string>
  <key>CFBundleIdentifier</key>   <string>com.example.kasalightcontrol</string>
  <key>CFBundleVersion</key>      <string>1.0</string>
  <key>CFBundlePackageType</key>  <string>APPL</string>
  <key>CFBundleExecutable</key>   <string>${APP_NAME}</string>
</dict>
</plist>
EOF

  echo "macOS .app bundle created at ${DIST_DIR}/KasaLightControl.app"
}

# Package the .app after building darwin binary
package_macos_app

echo -e "\nAll binaries built successfully. Find them in the '${DIST_DIR}' directory."
