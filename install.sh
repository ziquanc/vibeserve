#!/bin/sh
# VibeServe installer — downloads the latest release binary for your platform.
# Usage: curl -fsSL https://raw.githubusercontent.com/vibeserve/vibeserve/main/install.sh | sh

set -e

REPO="ziquanc/vibeserve"
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

# Download — try raw binary first (vibeserve_darwin_arm64), fall back to .tar.gz
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

FILENAME_RAW="${BINARY}_${OS}_${ARCH}"
FILENAME_TAR="${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz"
URL_RAW="https://github.com/${REPO}/releases/download/v${LATEST}/${FILENAME_RAW}"
URL_TAR="https://github.com/${REPO}/releases/download/v${LATEST}/${FILENAME_TAR}"

if curl -fsSL "$URL_RAW" -o "${TMPDIR}/${BINARY}" 2>/dev/null; then
  echo "Downloaded binary."
elif curl -fsSL "$URL_TAR" -o "${TMPDIR}/${FILENAME_TAR}" 2>/dev/null; then
  echo "Downloaded archive."
  tar -xzf "${TMPDIR}/${FILENAME_TAR}" -C "$TMPDIR"
else
  echo "Error: Could not download vibeserve for ${OS}/${ARCH}."
  echo "Check https://github.com/${REPO}/releases for available downloads."
  exit 1
fi

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
