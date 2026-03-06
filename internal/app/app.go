package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/tg"
	"github.com/kaosb/tgcli/internal/store"
	tgclient "github.com/kaosb/tgcli/internal/tg"
)

type Options struct {
	StoreDir string
	JSON     bool
}

type App struct {
	opts Options
	tg   *tgclient.Client
	db   *store.DB
}

func New(opts Options) (*App, error) {
	if opts.StoreDir == "" {
		return nil, fmt.Errorf("store dir is required")
	}
	if err := os.MkdirAll(opts.StoreDir, 0700); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}

	dbPath := filepath.Join(opts.StoreDir, "tgcli.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}

	tgClient, err := tgclient.New(tgclient.Options{StoreDir: opts.StoreDir})
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &App{opts: opts, tg: tgClient, db: db}, nil
}

func (a *App) Close() {
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) DB() *store.DB        { return a.db }
func (a *App) TG() *tgclient.Client { return a.tg }
func (a *App) StoreDir() string     { return a.opts.StoreDir }

// RunTG runs a function inside the Telegram client session.
func (a *App) RunTG(ctx context.Context, fn func(ctx context.Context) error) error {
	return a.tg.Run(ctx, fn)
}

// Login authenticates with Telegram.
func (a *App) Login(ctx context.Context, phone, code, password string) error {
	return a.tg.Login(ctx, tgclient.LoginOptions{
		Phone:    phone,
		Code:     code,
		Password: password,
	})
}

// SendText sends a text message to the given chat.
func (a *App) SendText(ctx context.Context, chatArg, message string) error {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return err
	}

	updates, err := a.tg.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  message,
		RandomID: time.Now().UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	// Try to extract the message ID from the response.
	msgID := 0
	if upd, ok := updates.(*tg.Updates); ok {
		for _, u := range upd.Updates {
			if newMsg, ok := u.(*tg.UpdateNewMessage); ok {
				if msg, ok := newMsg.Message.(*tg.Message); ok {
					msgID = msg.ID
				}
			}
		}
	}

	// Store in local DB.
	peerID := peerToID(peer)
	now := time.Now().UTC()
	_ = a.db.UpsertChat(peerID, peerKind(peer), chatArg, now)
	_ = a.db.UpsertMessage(store.UpsertMessageParams{
		ChatID:     peerID,
		ChatName:   chatArg,
		MsgID:      msgID,
		SenderID:   fmt.Sprintf("%d", a.tg.SelfID()),
		SenderName: "me",
		Timestamp:  now,
		FromMe:     true,
		Text:       message,
	})

	return nil
}

// SendFile sends a file to the given chat.
func (a *App) SendFile(ctx context.Context, chatArg, filePath, caption string) error {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return err
	}

	u := a.tg.Uploader()
	f, err := u.FromPath(ctx, filePath)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}

	_, err = a.tg.API().MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: peer,
		Media: &tg.InputMediaUploadedDocument{
			File:     f,
			MimeType: "application/octet-stream",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filepath.Base(filePath)},
			},
		},
		Message:  caption,
		RandomID: time.Now().UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("send file: %w", err)
	}

	return nil
}

// ListChats fetches dialogs from Telegram and returns them.
func (a *App) ListChats(ctx context.Context, chatType string, limit int) ([]store.Chat, error) {
	if limit <= 0 {
		limit = 50
	}

	dialogs, err := a.tg.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("get dialogs: %w", err)
	}

	var chats []store.Chat

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = a.extractChats(d.Dialogs, d.Users, d.Chats, chatType)
	case *tg.MessagesDialogsSlice:
		chats = a.extractChats(d.Dialogs, d.Users, d.Chats, chatType)
	}

	// Store in local DB.
	for _, c := range chats {
		_ = a.db.UpsertChat(c.PeerID, c.Kind, c.Name, c.LastMessageTS)
	}

	return chats, nil
}

// ListMessages fetches message history from Telegram.
func (a *App) ListMessages(ctx context.Context, chatArg string, limit int) ([]store.Message, error) {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	history, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}

	return a.extractMessages(history, chatArg, peer)
}

// MessageContext returns messages around a specific message.
func (a *App) MessageContext(ctx context.Context, chatArg string, msgID, before, after int) ([]store.Message, error) {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return nil, err
	}

	total := before + after + 1
	history, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		OffsetID: msgID + after + 1,
		Limit:    total,
	})
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}

	return a.extractMessages(history, chatArg, peer)
}

// SyncChat syncs message history for a specific chat to local DB.
func (a *App) SyncChat(ctx context.Context, chatArg string, onProgress func(fetched int)) error {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return err
	}

	peerID := peerToID(peer)
	offsetID := 0
	totalSynced := 0
	batchSize := 100

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		history, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    batchSize,
		})
		if err != nil {
			return fmt.Errorf("get history (offset %d): %w", offsetID, err)
		}

		messages, err := a.extractMessages(history, chatArg, peer)
		if err != nil {
			return err
		}

		if len(messages) == 0 {
			break
		}

		totalSynced += len(messages)
		if onProgress != nil {
			onProgress(totalSynced)
		}

		// The oldest message in this batch becomes the offset for the next.
		// extractMessages reverses to chronological, so first element is oldest.
		offsetID = messages[0].MsgID

		// Also update chat record.
		_ = a.db.UpsertChat(peerID, peerKind(peer), chatArg, messages[len(messages)-1].Timestamp)

		if len(messages) < batchSize {
			break
		}

		// Small delay to avoid flood wait.
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// SyncAllChats syncs all dialogs and their recent messages.
func (a *App) SyncAllChats(ctx context.Context, msgsPerChat int, onProgress func(chatName string, fetched int)) error {
	if msgsPerChat <= 0 {
		msgsPerChat = 100
	}

	// First, fetch all dialogs.
	chats, err := a.ListChats(ctx, "", 200)
	if err != nil {
		return fmt.Errorf("list chats: %w", err)
	}

	for _, chat := range chats {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		peer, err := a.peerFromChat(chat)
		if err != nil {
			continue
		}

		history, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:  peer,
			Limit: msgsPerChat,
		})
		if err != nil {
			// Skip chats we can't access.
			continue
		}

		messages, _ := a.extractMessages(history, chat.Name, peer)
		if onProgress != nil {
			onProgress(chat.Name, len(messages))
		}

		time.Sleep(300 * time.Millisecond)
	}

	return nil
}

// DownloadMedia downloads media from a specific message.
func (a *App) DownloadMedia(ctx context.Context, chatArg string, msgID int, outputDir string) (string, error) {
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return "", err
	}

	// Fetch the specific message.
	result, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		OffsetID: msgID + 1,
		Limit:    1,
	})
	if err != nil {
		return "", fmt.Errorf("get message: %w", err)
	}

	var tgMessages []tg.MessageClass
	switch m := result.(type) {
	case *tg.MessagesMessages:
		tgMessages = m.Messages
	case *tg.MessagesMessagesSlice:
		tgMessages = m.Messages
	case *tg.MessagesChannelMessages:
		tgMessages = m.Messages
	}

	for _, m := range tgMessages {
		msg, ok := m.(*tg.Message)
		if !ok || msg.ID != msgID {
			continue
		}

		if msg.Media == nil {
			return "", fmt.Errorf("message %d has no media", msgID)
		}

		return a.downloadMessageMedia(ctx, msg, outputDir)
	}

	return "", fmt.Errorf("message %d not found", msgID)
}

func (a *App) downloadMessageMedia(ctx context.Context, msg *tg.Message, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	d := a.tg.Downloader()

	switch media := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return "", fmt.Errorf("invalid document")
		}

		filename := fmt.Sprintf("%d_%d", msg.ID, doc.ID)
		for _, attr := range doc.Attributes {
			if f, ok := attr.(*tg.DocumentAttributeFilename); ok {
				filename = f.FileName
				break
			}
		}

		outputPath := filepath.Join(outputDir, filename)
		f, err := os.Create(outputPath)
		if err != nil {
			return "", fmt.Errorf("create file: %w", err)
		}
		defer f.Close()

		loc := &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		}

		_, err = d.Download(a.tg.API(), loc).Stream(ctx, f)
		if err != nil {
			os.Remove(outputPath)
			return "", fmt.Errorf("download document: %w", err)
		}

		// Update local DB with download path.
		_ = a.db.UpdateMessageLocalPath(peerToID(&tg.InputPeerUser{}), msg.ID, outputPath)

		return outputPath, nil

	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return "", fmt.Errorf("invalid photo")
		}

		// Get the largest size.
		var bestSize *tg.PhotoSize
		for _, s := range photo.Sizes {
			if ps, ok := s.(*tg.PhotoSize); ok {
				if bestSize == nil || ps.Size > bestSize.Size {
					bestSize = ps
				}
			}
		}
		if bestSize == nil {
			return "", fmt.Errorf("no photo sizes available")
		}

		filename := fmt.Sprintf("%d_%d.jpg", msg.ID, photo.ID)
		outputPath := filepath.Join(outputDir, filename)
		f, err := os.Create(outputPath)
		if err != nil {
			return "", fmt.Errorf("create file: %w", err)
		}
		defer f.Close()

		loc := &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     bestSize.Type,
		}

		_, err = d.Download(a.tg.API(), loc).Stream(ctx, f)
		if err != nil {
			os.Remove(outputPath)
			return "", fmt.Errorf("download photo: %w", err)
		}

		return outputPath, nil

	default:
		return "", fmt.Errorf("unsupported media type: %T", media)
	}
}

// ExportChat exports messages from a chat to a writer as JSON.
func (a *App) ExportChat(ctx context.Context, chatArg string, w io.Writer, fromDB bool) error {
	if fromDB {
		// Export from local DB.
		messages, err := a.db.ListMessages(store.ListMessagesParams{
			ChatID: chatArg,
			Limit:  100000,
		})
		if err != nil {
			return fmt.Errorf("list messages from DB: %w", err)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	// Export from Telegram API.
	peer, err := a.tg.ResolvePeer(ctx, chatArg)
	if err != nil {
		return err
	}

	var allMessages []store.Message
	offsetID := 0
	batchSize := 100

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		history, err := a.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    batchSize,
		})
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}

		messages, err := a.extractMessages(history, chatArg, peer)
		if err != nil {
			return err
		}

		if len(messages) == 0 {
			break
		}

		allMessages = append(allMessages, messages...)
		offsetID = messages[0].MsgID

		if len(messages) < batchSize {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(allMessages)
}

// --- helpers ---

func (a *App) peerFromChat(chat store.Chat) (tg.InputPeerClass, error) {
	return a.tg.ResolvePeer(context.Background(), chat.PeerID)
}

func (a *App) extractChats(dialogs []tg.DialogClass, users []tg.UserClass, chatEntities []tg.ChatClass, filterType string) []store.Chat {
	userMap := make(map[int64]*tg.User)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	chatMap := make(map[int64]tg.ChatClass)
	for _, c := range chatEntities {
		switch v := c.(type) {
		case *tg.Chat:
			chatMap[v.ID] = v
		case *tg.Channel:
			chatMap[v.ID] = v
		}
	}

	var result []store.Chat
	for _, d := range dialogs {
		dialog, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}

		var c store.Chat
		switch p := dialog.Peer.(type) {
		case *tg.PeerUser:
			if filterType != "" && filterType != "private" {
				continue
			}
			c.PeerID = fmt.Sprintf("%d", p.UserID)
			c.Kind = "private"
			if u, ok := userMap[p.UserID]; ok {
				c.Name = userName(u)
			}
		case *tg.PeerChat:
			if filterType != "" && filterType != "group" {
				continue
			}
			c.PeerID = fmt.Sprintf("-%d", p.ChatID)
			c.Kind = "group"
			if ch, ok := chatMap[p.ChatID]; ok {
				if chat, ok := ch.(*tg.Chat); ok {
					c.Name = chat.Title
				}
			}
		case *tg.PeerChannel:
			if filterType != "" && filterType != "channel" {
				continue
			}
			c.PeerID = fmt.Sprintf("-100%d", p.ChannelID)
			c.Kind = "channel"
			if ch, ok := chatMap[p.ChannelID]; ok {
				if channel, ok := ch.(*tg.Channel); ok {
					c.Name = channel.Title
					if !channel.Broadcast {
						c.Kind = "group" // supergroup
					}
				}
			}
		default:
			continue
		}

		result = append(result, c)
	}

	return result
}

func (a *App) extractMessages(messagesClass tg.MessagesMessagesClass, chatArg string, peer tg.InputPeerClass) ([]store.Message, error) {
	var tgMessages []tg.MessageClass
	var users []tg.UserClass

	switch m := messagesClass.(type) {
	case *tg.MessagesMessages:
		tgMessages = m.Messages
		users = m.Users
	case *tg.MessagesMessagesSlice:
		tgMessages = m.Messages
		users = m.Users
	case *tg.MessagesChannelMessages:
		tgMessages = m.Messages
		users = m.Users
	}

	userMap := make(map[int64]string)
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = userName(user)
		}
	}

	peerID := peerToID(peer)
	var messages []store.Message

	for _, m := range tgMessages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}

		sm := store.Message{
			ChatID:    peerID,
			ChatName:  chatArg,
			MsgID:     msg.ID,
			Timestamp: time.Unix(int64(msg.Date), 0).UTC(),
			FromMe:    msg.Out,
			Text:      msg.Message,
		}

		if msg.FromID != nil {
			if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
				sm.SenderID = fmt.Sprintf("%d", fromUser.UserID)
				sm.SenderName = userMap[fromUser.UserID]
			}
		}

		if msg.Media != nil {
			sm.MediaType = mediaTypeName(msg.Media)
		}

		messages = append(messages, sm)

		// Store in local DB.
		_ = a.db.UpsertMessage(store.UpsertMessageParams{
			ChatID:     sm.ChatID,
			ChatName:   sm.ChatName,
			MsgID:      sm.MsgID,
			SenderID:   sm.SenderID,
			SenderName: sm.SenderName,
			Timestamp:  sm.Timestamp,
			FromMe:     sm.FromMe,
			Text:       sm.Text,
			MediaType:  sm.MediaType,
		})
	}

	// Reverse to chronological order (API returns newest first).
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func userName(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	if name == "" {
		name = u.Username
	}
	return name
}

func peerToID(peer tg.InputPeerClass) string {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		return fmt.Sprintf("%d", p.UserID)
	case *tg.InputPeerChat:
		return fmt.Sprintf("-%d", p.ChatID)
	case *tg.InputPeerChannel:
		return fmt.Sprintf("-100%d", p.ChannelID)
	default:
		return "unknown"
	}
}

func peerKind(peer tg.InputPeerClass) string {
	switch peer.(type) {
	case *tg.InputPeerUser:
		return "private"
	case *tg.InputPeerChat:
		return "group"
	case *tg.InputPeerChannel:
		return "channel"
	default:
		return "unknown"
	}
}

func mediaTypeName(media tg.MessageMediaClass) string {
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaGeo:
		return "geo"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaVenue:
		return "venue"
	case *tg.MessageMediaGame:
		return "game"
	case *tg.MessageMediaGeoLive:
		return "geo_live"
	case *tg.MessageMediaPoll:
		return "poll"
	case *tg.MessageMediaWebPage:
		return "webpage"
	case *tg.MessageMediaStory:
		return "story"
	default:
		return "unknown"
	}
}
