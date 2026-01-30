#!/bin/bash
# FrankenDeploy Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/yoanbernabeu/frankendeploy/main/scripts/install.sh | sh

set -e

REPO="yoanbernabeu/frankendeploy"
BINARY="frankendeploy"
INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}FrankenDeploy Installer${NC}"
echo "========================"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

# Get latest version
echo "Fetching latest version..."
VERSION=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
    echo -e "${RED}Failed to fetch latest version${NC}"
    exit 1
fi

echo "Latest version: $VERSION"

# Build download URL
FILENAME="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

echo "Downloading ${FILENAME}..."
curl -sL "$DOWNLOAD_URL" -o "$TMP_DIR/$FILENAME"

if [ ! -f "$TMP_DIR/$FILENAME" ]; then
    echo -e "${RED}Download failed${NC}"
    exit 1
fi

# Extract
echo "Extracting..."
tar -xzf "$TMP_DIR/$FILENAME" -C "$TMP_DIR"

# Install
echo "Installing to $INSTALL_DIR..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
    chmod +x "$INSTALL_DIR/$BINARY"
else
    sudo mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
    sudo chmod +x "$INSTALL_DIR/$BINARY"
fi

# Verify installation
if command -v $BINARY &> /dev/null; then
    echo ""
    echo -e "${GREEN}✅ FrankenDeploy installed successfully!${NC}"
    echo ""
    $BINARY --version
    echo ""
    echo "Get started:"
    echo "  cd your-symfony-project"
    echo "  frankendeploy init"
else
    echo -e "${RED}Installation failed${NC}"
    exit 1
fi
