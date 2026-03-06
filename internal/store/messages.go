package store

import (
	"fmt"
	"strings"
	"time"
)

type UpsertMessageParams struct {
	ChatID       string
	ChatName     string
	MsgID        int
	SenderID     string
	SenderName   string
	Timestamp    time.Time
	FromMe       bool
	Text         string
	MediaType    string
	MediaCaption string
	Filename     string
	MimeType     string
	FileID       string
	FileSize     int64
}

func (d *DB) UpsertMessage(p UpsertMessageParams) error {
	_, err := d.sql.Exec(`
		INSERT INTO messages(chat_id, chat_name, msg_id, sender_id, sender_name, ts, from_me, text, media_type, media_caption, filename, mime_type, file_id, file_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, msg_id) DO UPDATE SET
			chat_name=COALESCE(NULLIF(excluded.chat_name,''), messages.chat_name),
			sender_id=excluded.sender_id,
			sender_name=COALESCE(NULLIF(excluded.sender_name,''), messages.sender_name),
			ts=excluded.ts,
			from_me=excluded.from_me,
			text=excluded.text,
			media_type=excluded.media_type,
			media_caption=excluded.media_caption,
			filename=COALESCE(NULLIF(excluded.filename,''), messages.filename),
			mime_type=COALESCE(NULLIF(excluded.mime_type,''), messages.mime_type),
			file_id=COALESCE(NULLIF(excluded.file_id,''), messages.file_id),
			file_size=CASE WHEN excluded.file_size>0 THEN excluded.file_size ELSE messages.file_size END
	`, p.ChatID, nilIfEmpty(p.ChatName), p.MsgID, nilIfEmpty(p.SenderID), nilIfEmpty(p.SenderName),
		unix(p.Timestamp), boolToInt(p.FromMe), nilIfEmpty(p.Text),
		nilIfEmpty(p.MediaType), nilIfEmpty(p.MediaCaption), nilIfEmpty(p.Filename),
		nilIfEmpty(p.MimeType), nilIfEmpty(p.FileID), p.FileSize,
	)
	return err
}

func nilIfEmpty(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

type ListMessagesParams struct {
	ChatID string
	Limit  int
	Before *time.Time
	After  *time.Time
}

func (d *DB) ListMessages(p ListMessagesParams) ([]Message, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	query := `
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''), ''
		FROM messages m
		LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE 1=1`
	var args []interface{}

	if strings.TrimSpace(p.ChatID) != "" {
		query += " AND m.chat_id = ?"
		args = append(args, p.ChatID)
	}
	if p.After != nil {
		query += " AND m.ts > ?"
		args = append(args, unix(*p.After))
	}
	if p.Before != nil {
		query += " AND m.ts < ?"
		args = append(args, unix(*p.Before))
	}

	query += " ORDER BY m.ts DESC LIMIT ?"
	args = append(args, p.Limit)
	return d.scanMessages(query, args...)
}

func (d *DB) GetMessage(chatID string, msgID int) (Message, error) {
	row := d.sql.QueryRow(`
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''), ''
		FROM messages m
		LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE m.chat_id = ? AND m.msg_id = ?
	`, chatID, msgID)
	return d.scanMessage(row)
}

func (d *DB) MessageContext(chatID string, msgID int, before, after int) ([]Message, error) {
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}

	target, err := d.GetMessage(chatID, msgID)
	if err != nil {
		return nil, fmt.Errorf("target message not found: %w", err)
	}

	beforeRows, err := d.scanMessages(`
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''), ''
		FROM messages m LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE m.chat_id = ? AND m.ts < ?
		ORDER BY m.ts DESC LIMIT ?
	`, chatID, unix(target.Timestamp), before)
	if err != nil {
		return nil, err
	}

	afterRows, err := d.scanMessages(`
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''), ''
		FROM messages m LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE m.chat_id = ? AND m.ts > ?
		ORDER BY m.ts ASC LIMIT ?
	`, chatID, unix(target.Timestamp), after)
	if err != nil {
		return nil, err
	}

	// Reverse before rows to chronological order.
	for i, j := 0, len(beforeRows)-1; i < j; i, j = i+1, j-1 {
		beforeRows[i], beforeRows[j] = beforeRows[j], beforeRows[i]
	}

	out := make([]Message, 0, len(beforeRows)+1+len(afterRows))
	out = append(out, beforeRows...)
	out = append(out, target)
	out = append(out, afterRows...)
	return out, nil
}

func (d *DB) scanMessages(query string, args ...interface{}) ([]Message, error) {
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var ts int64
		var fromMe int
		if err := rows.Scan(&m.ChatID, &m.ChatName, &m.MsgID, &m.SenderID, &m.SenderName, &ts, &fromMe, &m.Text, &m.MediaType, &m.Snippet); err != nil {
			return nil, err
		}
		m.Timestamp = fromUnix(ts)
		m.FromMe = fromMe != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) UpdateMessageLocalPath(chatID string, msgID int, localPath string) error {
	_, err := d.sql.Exec(`UPDATE messages SET local_path = ? WHERE chat_id = ? AND msg_id = ?`, localPath, chatID, msgID)
	return err
}

func (d *DB) CountMessages() (int64, error) {
	row := d.sql.QueryRow(`SELECT COUNT(1) FROM messages`)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (d *DB) CountChats() (int64, error) {
	row := d.sql.QueryRow(`SELECT COUNT(1) FROM chats`)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func (d *DB) scanMessage(row scannable) (Message, error) {
	var m Message
	var ts int64
	var fromMe int
	if err := row.Scan(&m.ChatID, &m.ChatName, &m.MsgID, &m.SenderID, &m.SenderName, &ts, &fromMe, &m.Text, &m.MediaType, &m.Snippet); err != nil {
		return Message{}, err
	}
	m.Timestamp = fromUnix(ts)
	m.FromMe = fromMe != 0
	return m, nil
}
