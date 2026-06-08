#!/usr/bin/env bash
# ============================================================
# Nahan Wizard — One-line Installer
# Usage: bash <(curl -fsSL https://raw.githubusercontent.com/YOUR_REPO/main/install.sh)
# ============================================================

set -e

CYAN='\033[1;36m'
GREEN='\033[1;32m'
RED='\033[1;31m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

OK="${GREEN}[+]${NC}"
ERR="${RED}[-]${NC}"
INFO="${CYAN}[i]${NC}"

REPO="YOUR_GITHUB_USERNAME/nahan-wizard"  # ← تغییر بده
BINARY_NAME="nahan-wizard"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l)  ARCH="arm" ;;
  *)
    echo -e " ${ERR} Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Termux detection
if [ -d "/data/data/com.termux" ]; then
  INSTALL_DIR="$PREFIX/bin"
  OS="linux"
fi

echo -e "\n${CYAN}${BOLD}"
cat << "EOF"
███╗   ██╗ █████╗ ██╗  ██╗ █████╗ ███╗   ██╗
████╗  ██║██╔══██╗██║  ██║██╔══██╗████╗  ██║
██╔██╗ ██║███████║███████║███████║██╔██╗ ██║
██║╚██╗██║██╔══██║██╔══██║██╔══██║██║╚██╗██║
██║ ╚████║██║  ██║██║  ██║██║  ██║██║ ╚████║
╚═╝  ╚═══╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝
EOF
echo -e "${NC}"
echo -e " ${INFO} Installing Nahan Wizard for ${BOLD}${OS}/${ARCH}${NC}...\n"

# Get latest release URL
LATEST_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  LATEST_URL="${LATEST_URL}.exe"
fi

# Download
TMP_FILE=$(mktemp)
echo -e " ${INFO} Downloading from GitHub releases..."

if command -v curl &>/dev/null; then
  curl -fsSL "$LATEST_URL" -o "$TMP_FILE"
elif command -v wget &>/dev/null; then
  wget -q "$LATEST_URL" -O "$TMP_FILE"
else
  echo -e " ${ERR} Neither curl nor wget found. Please install one and retry."
  exit 1
fi

# Install
chmod +x "$TMP_FILE"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP_FILE" "$INSTALL_DIR/$BINARY_NAME"
elif command -v sudo &>/dev/null; then
  sudo mv "$TMP_FILE" "$INSTALL_DIR/$BINARY_NAME"
else
  mkdir -p "$HOME/.local/bin"
  mv "$TMP_FILE" "$HOME/.local/bin/$BINARY_NAME"
  INSTALL_DIR="$HOME/.local/bin"
  echo -e " ${YELLOW}[!]${NC} Installed to $INSTALL_DIR — make sure it's in your PATH"
fi

echo -e " ${OK} Nahan Wizard installed to: ${CYAN}${INSTALL_DIR}/${BINARY_NAME}${NC}"
echo -e " ${OK} Run it with: ${BOLD}${BINARY_NAME}${NC}\n"
