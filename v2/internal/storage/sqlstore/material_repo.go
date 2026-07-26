package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type materialRepo struct{ *Store }

const materialSelect = `SELECT id, session_id, category, title, summary, body, source,
		mime_type, storage_ref, tags, created_at, updated_at
		FROM materials`

func (r materialRepo) List(ctx interface{}, sessionID, category, query string, limit, offset int) ([]domain.Material, error) {
	c := ctx.(context.Context)
	var (
		clauses []string
		args    []any
	)
	clauses = append(clauses, "session_id = ?")
	args = append(args, sessionID)
	if category != "" {
		clauses = append(clauses, "category = ?")
		args = append(args, category)
	}
	if query != "" {
		like := "%" + query + "%"
		clauses = append(clauses, "(title LIKE ? OR summary LIKE ? OR body LIKE ?)")
		args = append(args, like, like, like)
	}
	q := materialSelect + ` WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY updated_at DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
		if offset > 0 {
			q += ` OFFSET ?`
			args = append(args, offset)
		}
	} else if offset > 0 {
		q += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}
	rows, err := r.query(c, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Material{}
	for rows.Next() {
		m, err := scanMaterial(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (r materialRepo) Get(ctx interface{}, sessionID, id string) (*domain.Material, error) {
	row := r.queryRow(ctx.(context.Context), materialSelect+` WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanMaterial(row.Scan)
}

func (r materialRepo) Create(ctx interface{}, m *domain.Material) (*domain.Material, error) {
	c := ctx.(context.Context)
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	now := nowMillis()
	_, err := r.exec(c,
		`INSERT INTO materials (id, session_id, category, title, summary, body, source, mime_type, storage_ref, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.Category, m.Title, m.Summary, m.Body, m.Source, m.MimeType, m.StorageRef, marshalJSON(m.Tags), now, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, m.SessionID, m.ID)
}

func (r materialRepo) Update(ctx interface{}, sessionID, id string, m *domain.Material) (*domain.Material, error) {
	c := ctx.(context.Context)
	if m.Tags == nil {
		m.Tags = []string{}
	}
	_, err := r.exec(c,
		`UPDATE materials SET category = ?, title = ?, summary = ?, body = ?, source = ?,
			mime_type = ?, storage_ref = ?, tags = ?, updated_at = ?
		 WHERE session_id = ? AND id = ?`,
		m.Category, m.Title, m.Summary, m.Body, m.Source, m.MimeType, m.StorageRef, marshalJSON(m.Tags), nowMillis(), sessionID, id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, sessionID, id)
}

func (r materialRepo) Delete(ctx interface{}, sessionID, id string) error {
	c := ctx.(context.Context)
	res, err := r.exec(c, `DELETE FROM materials WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanMaterial(scan func(dest ...any) error) (*domain.Material, error) {
	var (
		m         domain.Material
		tags      string
		createdAt int64
		updatedAt int64
	)
	err := scan(&m.ID, &m.SessionID, &m.Category, &m.Title, &m.Summary, &m.Body, &m.Source,
		&m.MimeType, &m.StorageRef, &tags, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if e := json.Unmarshal([]byte(tags), &m.Tags); e != nil || m.Tags == nil {
		m.Tags = []string{}
	}
	m.CreatedAt = fromMillis(createdAt)
	m.UpdatedAt = fromMillis(updatedAt)
	return &m, nil
}
