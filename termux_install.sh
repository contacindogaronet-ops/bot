#!/data/data/com.termux/files/usr/bin/bash
# ==============================================================================
# JARGO TELEGRAM USERBOT - TERMUX ARM64 ONE-CLICK INSTALLER
# ==============================================================================
set -e

echo "=========================================================="
echo " [JARGO] Setting up Telegram Media Downloader on Termux"
echo "=========================================================="

# 1. Update packages and install prerequisites
echo "[+] Updating Termux package manager..."
pkg update -y && pkg upgrade -y
pkg install -y golang git termux-tools make openssl

# 2. Grant Termux storage access to save to /sdcard/Download
echo "[+] Setting up storage permission..."
termux-setup-storage

# 3. Create downloads folder
DOWNLOAD_DIR="$HOME/storage/downloads/TelegramUserbot"
mkdir -p "$DOWNLOAD_DIR"
echo "[+] Download directory prepared: $DOWNLOAD_DIR"

# 4. Check .env file
if [ ! -f ".env" ]; then
    echo "[!] .env file not found. Copying from .env.example..."
    cp .env.example .env
    echo "[!] Please edit .env with your TG_APP_ID and TG_APP_HASH from https://my.telegram.org"
fi

# 5. Build pure Go binary
echo "[+] Compiling native ARM64 binary..."
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o jargo-userbot main.go

echo "=========================================================="
echo " [✓] Build Successful!"
echo " To run in foreground: ./jargo-userbot"
echo " To run in background: nohup ./jargo-userbot > bot.log 2>&1 &"
echo "=========================================================="
