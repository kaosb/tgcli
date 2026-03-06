package store

import (
	"database/sql"
	"errors"
	"time"
)

type Chat struct {
	PeerID        string    `json:"peer_id"`
	Kind          string    `json:"kind"` // private, group, channel
	Name          string    `json:"name"`
	LastMessageTS time.Time `json:"last_message_ts"`
}

type Message struct {
	ChatID     string    `json:"chat_id"`
	ChatName   string    `json:"chat_name"`
	MsgID      int       `json:"msg_id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Timestamp  time.Time `json:"timestamp"`
	FromMe     bool      `json:"from_me"`
	Text       string    `json:"text"`
	MediaType  string    `json:"media_type,omitempty"`
	Snippet    string    `json:"snippet,omitempty"`
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func fromUnix(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (d *DB) HasFTS() bool { return d.ftsEnabled }

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
