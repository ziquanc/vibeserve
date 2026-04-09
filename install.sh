#!/bin/sh
# VibeServe installer — downloads the latest release binary for your platform.
# Usage: curl -fsSL https://raw.githubusercontent.com/vibeserve/vibeserve/main/install.sh | sh

set -e

REPO="vibeserve/vibeserve"
INSTALL_DIR="/usr/local/bin"
BINARY="vibeserve"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Error: Unsupported operating system: $OS"
    echo "VibeServe supports macOS and Linux."
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64)  ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Get latest release tag
echo "Fetching latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Error: Could not determine latest version."
  echo "Check https://github.com/${REPO}/releases manually."
  exit 1
fi

echo "Installing vibeserve v${LATEST} (${OS}/${ARCH})..."

# Download
FILENAME="${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${LATEST}/${FILENAME}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -fsSL "$URL" -o "${TMPDIR}/${FILENAME}"

# Extract
tar -xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "  vibeserve v${LATEST} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "  Get started:"
echo "    vibeserve"
echo ""
