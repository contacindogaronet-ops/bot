# JARGO Telegram Media Downloader Userbot

> **Target:** Android Termux (ARM64) / Linux Server  
> **Engine:** Pure Go MTProto (`github.com/gotd/td`) — Zero CGO, Zero TDLib, Zero-Alloc Streaming  
> **Trigger:** Single character `"d"` reply to any protected/restricted Telegram message.

---

## ⚡ Key Highlights

1. **Pure Go MTProto Implementation:**
   - Powered by `gotd/td` with zero CGO and zero external dynamic library dependencies.
   - Compiles down to a single static ARM64/x86 binary with `-ldflags="-s -w"`.
2. **Restricted Media Direct Stream:**
   - Bypasses client-side UI restriction flags (`noforwards`) by querying original `InputDocumentFileLocation` / `InputPhotoFileLocation` directly over raw MTProto transport.
   - Streamed chunk-by-chunk directly into disk using `telegram/downloader` without caching gigabytes in RAM (zero OOM on Android).
3. **Session Persistence:**
   - Implements `session.Storage` with atomic file writes to `session.json`.
   - Your `AuthKey` and Data Center state survive device restarts without prompting for phone login again.
4. **Zerolog Observability:**
   - Zero standard `fmt` / `log` prints.
   - Microsecond-precision structured logging with level filtering and console color support.
5. **Interactive Single-Letter Trigger:**
   - Reply to any video, audio, voice note, photo, or document in any chat with `d`.
   - The userbot displays live chunk download progress (percentage, speed, MBs) and updates the message upon completion with the absolute file path.

---

## 📂 Directory Structure (`/bot`)

```
/bot
├── .env.example          # Environment configuration template
├── Makefile              # Native & cross-compilation automation
├── README.md             # Architecture & manual
├── go.mod                # Module definition with gotd/td & zerolog
├── main.go               # Entry point & MTProto lifecycle
├── termux_install.sh     # One-click bootstrap script for Android Termux
├── config/
│   └── config.go         # Zero-alloc configuration loader
├── session/
│   └── storage.go        # Atomic file-backed session persistence
├── downloader/
│   └── downloader.go     # Chunked zero-alloc disk streaming & retry engine
├── handler/
│   └── dispatcher.go     # Message interceptor for "d" trigger
└── utils/
    └── logger.go         # Structured Zerolog logger setup
```

---

## 🚀 Quickstart on Android (Termux ARM64)

### 1. Install Prerequisites in Termux
```bash
pkg update -y && pkg upgrade -y
pkg install -y golang git make
termux-setup-storage
```

### 2. Configure Credentials
Obtain your `API_ID` and `API_HASH` from [my.telegram.org](https://my.telegram.org):
```bash
cp .env.example .env
nano .env
```
Fill in:
```env
TG_APP_ID=12345678
TG_APP_HASH=0123456789abcdef0123456789abcdef
TG_DOWNLOAD_DIR=/data/data/com.termux/files/home/storage/downloads/TelegramUserbot
```

### 3. Build and Run
```bash
# Build native ARM64 binary
make build

# Launch userbot (first run prompts for phone number & code)
./bin/jargo-userbot
```

### 4. How to Use in Telegram
1. Open any channel, group, or private chat containing restricted/unforwardable media.
2. Reply directly to the message with: `d`
3. Watch the userbot edit the status in real-time and save the media straight to your downloads directory!
