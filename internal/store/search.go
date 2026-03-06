package store

import (
	"fmt"
	"strings"
)

type SearchMessagesParams struct {
	Query  string
	ChatID string
	Limit  int
}

func (d *DB) SearchMessages(p SearchMessagesParams) ([]Message, error) {
	if strings.TrimSpace(p.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if d.ftsEnabled {
		return d.searchFTS(p)
	}
	return d.searchLIKE(p)
}

func (d *DB) searchFTS(p SearchMessagesParams) ([]Message, error) {
	query := `
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''),
		       snippet(messages_fts, 0, '[', ']', '…', 12)
		FROM messages_fts
		JOIN messages m ON messages_fts.rowid = m.rowid
		LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE messages_fts MATCH ?`
	args := []interface{}{p.Query}

	if strings.TrimSpace(p.ChatID) != "" {
		query += " AND m.chat_id = ?"
		args = append(args, p.ChatID)
	}

	query += " ORDER BY bm25(messages_fts) LIMIT ?"
	args = append(args, p.Limit)
	return d.scanMessages(query, args...)
}

func (d *DB) searchLIKE(p SearchMessagesParams) ([]Message, error) {
	needle := "%" + p.Query + "%"
	query := `
		SELECT m.chat_id, COALESCE(c.name,''), m.msg_id, COALESCE(m.sender_id,''), COALESCE(m.sender_name,''), m.ts, m.from_me, COALESCE(m.text,''), COALESCE(m.media_type,''), ''
		FROM messages m
		LEFT JOIN chats c ON c.peer_id = m.chat_id
		WHERE (LOWER(m.text) LIKE LOWER(?) OR LOWER(m.media_caption) LIKE LOWER(?) OR LOWER(COALESCE(m.sender_name,'')) LIKE LOWER(?))`
	args := []interface{}{needle, needle, needle}

	if strings.TrimSpace(p.ChatID) != "" {
		query += " AND m.chat_id = ?"
		args = append(args, p.ChatID)
	}

	query += " ORDER BY m.ts DESC LIMIT ?"
	args = append(args, p.Limit)
	return d.scanMessages(query, args...)
}
