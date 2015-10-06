package provider

import (
	"database/sql"
	"encoding/json"

	"github.com/cznic/ql"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

func init() {
	ql.RegisterDriver()
}

type SQLSilences struct {
	db *sql.DB
}

func (s *SQLSilences) Close() error {
	return s.db.Close()
}

func NewSQLSilences() (*SQLSilences, error) {
	db, err := sql.Open("ql", "data/am.db")
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createSilencesTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &SQLSilences{db: db}, nil
}

const createSilencesTable = `
CREATE TABLE IF NOT EXISTS silences (
	matchers   string,
	starts_at  time,
	ends_at    time,
	created_at time,
	created_by string,
	comment    string
);
CREATE INDEX IF NOT EXISTS silences_end ON silences (starts_at);
CREATE INDEX IF NOT EXISTS silences_end ON silences (ends_at);
CREATE INDEX IF NOT EXISTS silences_id  ON silences (id());
`

func (s *SQLSilences) Mutes(lset model.LabelSet) bool {
	sils, err := s.All()
	if err != nil {
		log.Errorf("retrieving silences failed: %s", err)
		// In doubt, do not silence anything.
		return false
	}

	for _, sil := range sils {
		if sil.Mutes(lset) {
			return true
		}
	}
	return false
}

func (s *SQLSilences) All() ([]*types.Silence, error) {
	rows, err := s.db.Query(`SELECT id(), matchers, starts_at, ends_at, created_at, created_by, comment FROM silences ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var silences []*types.Silence

	for rows.Next() {
		var (
			sil      types.Silence
			matchers string
		)

		if err := rows.Scan(&sil.ID, &matchers, &sil.StartsAt, &sil.EndsAt, &sil.CreatedAt, &sil.CreatedBy, &sil.Comment); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(matchers), &sil.Matchers); err != nil {
			return nil, err
		}

		silences = append(silences, &sil)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return silences, nil
}

func (s *SQLSilences) Set(sil *types.Silence) (uint64, error) {
	mb, err := json.Marshal(sil.Matchers)
	if err != nil {
		return 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(`INSERT INTO silences VALUES ($1, $2, $3, $4, $5, $6)`,
		string(mb),
		sil.StartsAt,
		sil.EndsAt,
		sil.CreatedAt,
		sil.CreatedBy,
		sil.Comment,
	)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	sid, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	tx.Commit()

	return uint64(sid), nil
}

func (s *SQLSilences) Del(sid uint64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM silences WHERE id() == $1`, sid); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()

	return nil
}

func (s *SQLSilences) Get(sid uint64) (*types.Silence, error) {
	row := s.db.QueryRow(`SELECT id(), matchers, starts_at, ends_at, created_at, created_by, comment FROM silences WHERE id() == $1`, sid)

	var (
		sil      types.Silence
		matchers string
	)
	err := row.Scan(&sil.ID, &matchers, &sil.StartsAt, &sil.EndsAt, &sil.CreatedAt, &sil.CreatedBy, &sil.Comment)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(matchers), &sil.Matchers); err != nil {
		return nil, err
	}

	return &sil, nil
}
