package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type companionRepo struct{ *Store }

func (r companionRepo) Get(ctx context.Context, sessionID string) (*domain.CompanionMemory, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, key_facts, conversation_history, updated_at
		 FROM companion_memory WHERE session_id = ?`, sessionID)
	var (
		m         domain.CompanionMemory
		keyFacts  string
		history   string
		updatedAt int64
	)
	err := row.Scan(&m.ID, &m.SessionID, &keyFacts, &history, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal([]byte(keyFacts), &m.KeyFacts) != nil || m.KeyFacts == nil {
		m.KeyFacts = []string{}
	}
	if json.Unmarshal([]byte(history), &m.ConversationHistory) != nil || m.ConversationHistory == nil {
		m.ConversationHistory = []domain.Message{}
	}
	m.UpdatedAt = fromMillis(updatedAt)
	return &m, nil
}

func (r companionRepo) Upsert(ctx context.Context, sessionID string, history []domain.Message, keyFacts []string) error {
	if history == nil {
		history = []domain.Message{}
	}
	if keyFacts == nil {
		keyFacts = []string{}
	}
	historyJSON := marshalJSON(history)
	keyFactsJSON := marshalJSON(keyFacts)
	now := nowMillis()

	res, err := r.exec(ctx,
		`UPDATE companion_memory SET conversation_history = ?, key_facts = ?, updated_at = ? WHERE session_id = ?`,
		historyJSON, keyFactsJSON, now, sessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO companion_memory (id, session_id, key_facts, conversation_history, updated_at)
			 VALUES (?, ?, ?, ?, ?)`,
			uuid.NewString(), sessionID, keyFactsJSON, historyJSON, now)
	}
	return err
}
