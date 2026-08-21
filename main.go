package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/jargo/telegram-downloader-userbot/config"
	"github.com/jargo/telegram-downloader-userbot/downloader"
	"github.com/jargo/telegram-downloader-userbot/handler"
	"github.com/jargo/telegram-downloader-userbot/session"
	"github.com/jargo/telegram-downloader-userbot/utils"
	"golang.org/x/term"
)

var (
	Version   = "1.0.0"
	BuildTime = "dev"
)

// termAuth implements auth.UserAuthenticator reading safely from stdin without fmt/log leaks.
type termAuth struct {
	phone string
}

func (a termAuth) Phone(_ context.Context) (string, error) {
	if a.phone != "" {
		return a.phone, nil
	}
	os.Stdout.WriteString("Enter Phone Number (+international): ")
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (a termAuth) Password(_ context.Context) (string, error) {
	os.Stdout.WriteString("Enter 2FA Cloud Password: ")
	bytePwd, err := term.ReadPassword(int(syscall.Stdin))
	os.Stdout.WriteString("\n")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePwd)), nil
}

func (a termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	os.Stdout.WriteString("Enter Telegram Login Code received in app: ")
	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func (a termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign-up not supported by userbot engine; use an existing account")
}

func (a termAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func main() {
	// Root Zerolog initialization
	log := utils.InitLogger("info", true)

	log.Info().
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("target", "Termux ARM64 / Linux Pure Go").
		Msg("[JARGO USERBOT ENGINE ACTIVE] Ready to build session-persistent Telegram media downloader inside /bot.")

	// Load configuration
	cfg, err := config.LoadConfig(log)
	if err != nil {
		log.Fatal().Err(err).Msg("Configuration error")
		return
	}

	// Reconfigure log level if specified in config
	log = utils.InitLogger(cfg.LogLevel, cfg.LogPretty)

	// Context with interrupt trap for graceful MTProto disconnect
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize persistent session storage
	sessionStorage, err := session.NewFileStorage(cfg.SessionPath, log)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.SessionPath).Msg("Failed to initialize session storage")
		return
	}

	// Updates manager dispatcher
	updateDispatcher := tg.NewUpdateDispatcher()
	gaps := updates.New(updates.Config{
		Handler: updateDispatcher,
		Logger:  nil,
	})

	// Client options with pure Go MTProto engine
	opts := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  gaps,
		Middlewares:    nil,
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, opts)

	// Run client runner
	if err := client.Run(ctx, func(ctx context.Context) error {
		rawAPI := client.API()

		// 1. Authenticate or restore persistent session
		authFlow := auth.NewFlow(
			termAuth{phone: cfg.PhoneNumber},
			auth.SendCodeOptions{},
		)

		status, err := client.Auth().Status(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get auth status")
			return err
		}

		if !status.Authorized {
			log.Info().Msg("🔑 Starting interactive Telegram authentication (session will be saved permanently)...")
			if err := client.Auth().IfNecessary(ctx, authFlow); err != nil {
				log.Error().Err(err).Msg("Authentication failed")
				return err
			}
			log.Info().Msg("✅ Successfully authenticated! Persistent session saved to disk.")
		} else {
			log.Info().Msg("⚡ Restored existing persistent session from disk without phone prompt")
		}

		// 2. Fetch self user info
		selfUser, err := client.Self(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch self user profile")
			return err
		}

		log.Info().
			Int64("user_id", selfUser.ID).
			Str("username", selfUser.Username).
			Str("first_name", selfUser.FirstName).
			Str("trigger_command", cfg.TriggerCmd).
			Str("downloads_dir", cfg.DownloadDir).
			Msg("🚀 JARGO Userbot is ONLINE and listening for 'd' command replies!")

		// 3. Initialize downloader and message handler
		dl, err := downloader.New(rawAPI, cfg.DownloadDir, cfg.ChunkSize, cfg.MaxConcurrentDownloads, log)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize chunked downloader")
			return err
		}

		msgHandler := handler.NewHandler(rawAPI, cfg, dl, log)
		msgHandler.SetSelfID(selfUser.ID)

		// 4. Hook update events
		updateDispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
			if msg, ok := update.Message.(*tg.Message); ok {
				return msgHandler.HandleNewMessage(ctx, e, msg)
			}
			return nil
		})

		updateDispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
			if msg, ok := update.Message.(*tg.Message); ok {
				return msgHandler.HandleNewMessage(ctx, e, msg)
			}
			return nil
		})

		// 5. Block on gaps / updates loop until context cancellation
		return gaps.Run(ctx, rawAPI, selfUser.ID, updates.AuthOptions{
			OnStart: func(ctx context.Context) {
				log.Info().Msg("🔄 Telegram MTProto gaps update engine active")
			},
		})
	}); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("JARGO Userbot execution stopped with error")
	}

	log.Info().Msg("🛑 JARGO Userbot stopped gracefully. Session preserved.")
}
