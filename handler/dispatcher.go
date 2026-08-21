package handler

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"github.com/jargo/telegram-downloader-userbot/config"
	"github.com/jargo/telegram-downloader-userbot/downloader"
	"github.com/rs/zerolog"
)

// Handler processes Telegram updates and handles the "d" command on replied media.
type Handler struct {
	client     *tg.Client
	cfg        *config.Config
	downloader *downloader.Downloader
	log        zerolog.Logger
	selfID     int64
	selfMux    sync.RWMutex
}

// NewHandler creates a new command dispatcher.
func NewHandler(client *tg.Client, cfg *config.Config, dl *downloader.Downloader, log zerolog.Logger) *Handler {
	return &Handler{
		client:     client,
		cfg:        cfg,
		downloader: dl,
		log:        log.With().Str("component", "handler").Logger(),
	}
}

// SetSelfID registers the authenticated user's ID to filter commands.
func (h *Handler) SetSelfID(id int64) {
	h.selfMux.Lock()
	defer h.selfMux.Unlock()
	h.selfID = id
}

func (h *Handler) getSelfID() int64 {
	h.selfMux.RLock()
	defer h.selfMux.RUnlock()
	return h.selfID
}

// HandleNewMessage intercepts incoming messages and processes the "d" download trigger.
func (h *Handler) HandleNewMessage(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if msg == nil {
		return nil
	}

	text := strings.TrimSpace(msg.Message)
	if text != h.cfg.TriggerCmd {
		return nil
	}

	// Validate sender is either self or in private/allowed context
	fromID, ok := getPeerID(msg.FromID)
	if ok && fromID != h.getSelfID() && !msg.Out {
		// Optional security: only allow self (userbot owner)
		h.log.Debug().Int64("from_id", fromID).Int64("self_id", h.getSelfID()).Msg("Ignoring trigger from other user")
		return nil
	}

	// Must be a reply to another message
	if msg.ReplyTo == nil {
		h.log.Debug().Int("msg_id", msg.ID).Msg("Trigger command 'd' ignored: not a reply")
		return nil
	}

	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || replyHeader.ReplyToMsgID == 0 {
		return nil
	}

	h.log.Info().
		Int("cmd_msg_id", msg.ID).
		Int("reply_to_id", replyHeader.ReplyToMsgID).
		Msg("⚡ Intercepted 'd' download command on replied message")

	// Execute download in background goroutine to not block MTProto updates pump
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		if err := h.processDownload(bgCtx, msg, replyHeader.ReplyToMsgID); err != nil {
			h.log.Error().Err(err).Int("target_msg_id", replyHeader.ReplyToMsgID).Msg("Download pipeline failed")
			h.editStatus(bgCtx, msg, fmt.Sprintf("❌ [JARGO Error] %v", err.Error()))
		}
	}()

	return nil
}

// processDownload fetches the restricted message payload and orchestrates the chunked download.
func (h *Handler) processDownload(ctx context.Context, cmdMsg *tg.Message, targetMsgID int) error {
	peer := msgPeerToInputPeer(cmdMsg.PeerID)
	if peer == nil {
		return fmt.Errorf("unable to resolve peer for message %d", cmdMsg.ID)
	}

	// 1. Fetch original message (this bypasses UI restriction flags because we query direct chat history)
	targetMsg, err := h.fetchMessage(ctx, peer, targetMsgID)
	if err != nil {
		return fmt.Errorf("failed to fetch target message: %w", err)
	}

	if targetMsg.Media == nil {
		return fmt.Errorf("replied message contains no downloadable media")
	}

	// 2. Resolve media target
	mediaTarget, err := h.downloader.ResolveMedia(targetMsg.Media)
	if err != nil {
		return fmt.Errorf("failed to resolve media: %w", err)
	}

	// 3. Update status message
	sizeStr := formatBytes(mediaTarget.Size)
	h.editStatus(ctx, cmdMsg, fmt.Sprintf("⚡ [JARGO] Downloading %s (%s)...", mediaTarget.FileName, sizeStr))

	lastEdit := time.Now()
	// 4. Stream to disk with real-time progress
	savedPath, err := h.downloader.DownloadStreams(ctx, mediaTarget, func(p downloader.DownloadProgress) {
		// Update Telegram status text periodically
		if time.Since(lastEdit) >= 3*time.Second {
			lastEdit = time.Now()
			speedStr := formatBytes(p.SpeedBytesSec) + "/s"
			progressText := fmt.Sprintf("⚡ [JARGO] Downloading: %s\n📊 Progress: %.1f%% (%s / %s)\n🚀 Speed: %s",
				mediaTarget.FileName,
				p.Percentage,
				formatBytes(p.DownloadedBytes),
				formatBytes(p.TotalBytes),
				speedStr,
			)
			h.editStatus(ctx, cmdMsg, progressText)
		}
	})

	if err != nil {
		return err
	}

	// 5. Success status
	absPath, _ := filepath.Abs(savedPath)
	successMsg := fmt.Sprintf("✅ [JARGO Done] Saved restricted media!\n📁 File: %s\n💾 Size: %s\n📍 Path: `%s`",
		mediaTarget.FileName,
		sizeStr,
		absPath,
	)
	h.editStatus(ctx, cmdMsg, successMsg)

	return nil
}

func (h *Handler) fetchMessage(ctx context.Context, peer tg.InputPeerClass, msgID int) (*tg.Message, error) {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		res, err := h.client.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  p.ChannelID,
				AccessHash: p.AccessHash,
			},
			ID: []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
		})
		if err != nil {
			return nil, err
		}
		return extractMessageFromClass(res, msgID)

	default:
		res, err := h.client.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}})
		if err != nil {
			return nil, err
		}
		return extractMessageFromClass(res, msgID)
	}
}

func extractMessageFromClass(res tg.MessagesMessagesClass, msgID int) (*tg.Message, error) {
	var msgs []tg.MessageClass
	switch m := res.(type) {
	case *tg.MessagesMessages:
		msgs = m.Messages
	case *tg.MessagesMessagesSlice:
		msgs = m.Messages
	case *tg.MessagesChannelMessages:
		msgs = m.Messages
	default:
		return nil, fmt.Errorf("unexpected messages response type: %T", res)
	}

	for _, item := range msgs {
		if msg, ok := item.(*tg.Message); ok && msg.ID == msgID {
			return msg, nil
		}
	}
	return nil, fmt.Errorf("message ID %d not found", msgID)
}

func (h *Handler) editStatus(ctx context.Context, msg *tg.Message, newText string) {
	peer := msgPeerToInputPeer(msg.PeerID)
	if peer == nil {
		return
	}

	_, err := h.client.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      msg.ID,
		Message: newText,
	})
	if err != nil {
		h.log.Debug().Err(err).Int("msg_id", msg.ID).Msg("Failed to edit trigger message status")
	}
}

func msgPeerToInputPeer(peer tg.PeerClass) tg.InputPeerClass {
	if peer == nil {
		return nil
	}
	switch p := peer.(type) {
	case *tg.PeerUser:
		return &tg.InputPeerUser{UserID: p.UserID}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}
	case *tg.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: p.ChannelID}
	default:
		return nil
	}
}

func getPeerID(peer tg.PeerClass) (int64, bool) {
	if peer == nil {
		return 0, false
	}
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID, true
	case *tg.PeerChat:
		return p.ChatID, true
	case *tg.PeerChannel:
		return p.ChannelID, true
	default:
		return 0, false
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
