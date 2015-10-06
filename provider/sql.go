package provider

import (
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/cznic/ql"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

func init() {
	ql.RegisterDriver()
}

type SQLNotifyInfo struct {
	db *sql.DB
}

func NewSQLNotifyInfo(file string) (*SQLNotifyInfo, error) {
	db, err := sql.Open("ql", file)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createNotifyInfoTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &SQLNotifyInfo{db: db}, nil
}

const createNotifyInfoTable = `
CREATE TABLE IF NOT EXISTS notify_info (
	alert       uint64
	destination string
	resolved    bool
	delivered   bool
	timestamp   time
);
CREATE UNIQUE INDEX IF NOT EXISTS notify_alert
	ON notify_info (alert, destination);
CREATE INDEX IF NOT EXISTS notify_delivered
	ON notify_info (delivered);
`

func (n *SQLNotifyInfo) Get(dest string, fps ...model.Fingerprint) ([]*types.Notify, error) {
	var result []*types.Notify

	for _, fp := range fps {
		row := n.db.QueryRow(`
			SELECT alert, destination, resolved, delivered, timestamp
			FROM notify_info
			WHERE destination == $1 AND fingerprint == $1
		`, fp)

		var ni types.Notify
		if err := row.Scan(&ni.Alert, &ni.SendTo, &ni.Resolved, &ni.Delivered, &ni.Timestamp); err != nil {
			return nil, err
		}

		result = append(result, &ni)
	}

	return result, nil
}

func (n *SQLNotifyInfo) Set(ns ...*types.Notify) error {
	tx, err := n.db.Begin()
	if err != nil {
		return err
	}

	insert, err := tx.Prepare(`
		INSERT INTO notify_info(alert, destination, resolved, delivered, timestamp)
		VALUES ($1, $2, $3, $4, $5);
	`)
	if err != nil {
		return err
	}
	defer insert.Close()

	del, err := tx.Prepare(`
		DELETE FROM notify_info
		WHERE alert == $1 AND destination == $2
	`)
	if err != nil {
		return err
	}
	defer del.Close()

	for _, ni := range ns {
		if _, err := del.Exec(ni.Alert, ni.SendTo); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := insert.Exec(ni.Alert, ni.SendTo, ni.Resolved, ni.Delivered, ni.Timestamp); err != nil {
			tx.Rollback()
			return err
		}
	}

	tx.Commit()

	return nil
}

type SQLAlerts struct {
	db *sql.DB

	mtx       sync.RWMutex
	listeners map[int]chan *types.Alert
	next      int
}

func NewSQLAlerts(file string) (*SQLAlerts, error) {
	db, err := sql.Open("ql", file)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createAlertsTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &SQLAlerts{
		db:        db,
		listeners: map[int]chan *types.Alert{},
	}, nil
}

const createAlertsTable = `
CREATE TABLE IF NOT EXISTS alerts (
	fingerprint uint64
	labels      string
	annotations string
	starts_at   time
	ends_at     time
	updated_at  time
	timeout     bool
);
CREATE INDEX IF NOT EXISTS alerts_start ON alerts (starts_at);
CREATE INDEX IF NOT EXISTS alerts_end   ON alerts (ends_at);
`

func (a *SQLAlerts) Subscribe() AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)
	alerts, err := a.getPending()
	if err != nil {
		panic(err)
	}

	i := a.next
	a.next++

	a.listeners[i] = ch

	go func() {
		defer func() {
			a.mtx.Lock()
			delete(a.listeners, i)
			close(ch)
			a.mtx.Unlock()
		}()

		for _, a := range alerts {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}

		<-done
	}()

	return memAlertIterator{
		ch:   ch,
		done: done,
	}
}

func (a *SQLAlerts) GetPending() AlertIterator {
	return nil
}

func (a *SQLAlerts) getPending() ([]*types.Alert, error) {
	rows, err := a.db.Query(`
		SELECT labels, annotations, starts_at, ends_at, updated_at, timeout
		FROM alerts
	`)
	if err != nil {
		return nil, err
	}

	var alerts []*types.Alert
	for rows.Next() {
		var (
			labels      string
			annotations string
			al          types.Alert
		)
		if err := rows.Scan(&labels, &annotations, &al.StartsAt, &al.EndsAt, &al.UpdatedAt, &al.Timeout); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(labels), &al.Labels); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(annotations), &al.Annotations); err != nil {
			return nil, err
		}

		alerts = append(alerts, &al)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (a *SQLAlerts) Get(model.Fingerprint) (*types.Alert, error) {
	return nil, nil
}

func (a *SQLAlerts) Put(alerts ...*types.Alert) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}

	overlap, err := tx.Prepare(`
		SELECT id(), annotations, starts_at, ends_at, updated_at, timeout FROM alerts
		WHERE fingerprint = $1 AND $2 =< ends_at OR $3 >= starts_at
	`)
	if err != nil {
		return err
	}
	defer overlap.Close()

	delOverlap, err := tx.Prepare(`
		DELETE FROM alerts WHERE id() IN (
			SELECT id() FROM alerts
			WHERE fingerprint = $1 AND $2 =< ends_at OR $3 >= starts_at
		)
	`)
	if err != nil {
		return err
	}
	defer delOverlap.Close()

	insert, err := tx.Prepare(`
		INSERT INTO alerts(fingerprint, labels, annotations, starts_at, ends_at, updated_at, timeout)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`)
	if err != nil {
		return err
	}
	defer insert.Close()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		// Retrieve all overlapping alerts and delete them.
		olaps, err := overlap.Query(fp, alert.StartsAt, alert.EndsAt)
		if err != nil {
			tx.Rollback()
			return err
		}

		var (
			overlapIDs []int64
			merges     []*types.Alert
		)
		for olaps.Next() {
			var (
				id  int64
				na  types.Alert
				ann string
			)
			if err := olaps.Scan(&id, &ann, &na.StartsAt, &na.EndsAt, &na.UpdatedAt, &na.Timeout); err != nil {
				tx.Rollback()
				return err
			}
			if err := json.Unmarshal([]byte(ann), &na.Annotations); err != nil {
				tx.Rollback()
				return err
			}
			na.Labels = alert.Labels

			merges = append(merges, &na)
			overlapIDs = append(overlapIDs, id)
		}
		if err := olaps.Err(); err != nil {
			tx.Rollback()
			return err
		}

		// Merge them.
		for _, ma := range merges {
			alert = alert.Merge(ma)
		}

		// Delete the old ones.
		if _, err := delOverlap.Exec(fp, alert.StartsAt, alert.EndsAt); err != nil {
			tx.Rollback()
			return err
		}

		// Insert the final alert.
		labels, err := json.Marshal(alert.Labels)
		if err != nil {
			tx.Rollback()
			return err
		}
		annotations, err := json.Marshal(alert.Annotations)
		if err != nil {
			tx.Rollback()
			return err
		}

		_, err = insert.Exec(
			fp,
			string(labels),
			string(annotations),
			alert.StartsAt,
			alert.EndsAt,
			alert.UpdatedAt,
			alert.Timeout,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	tx.Commit()

	return nil
}

type SQLSilences struct {
	db *sql.DB
}

func (s *SQLSilences) Close() error {
	return s.db.Close()
}

func NewSQLSilences(file string) (*SQLSilences, error) {
	db, err := sql.Open("ql", file)
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
CREATE INDEX IF NOT EXISTS silences_start ON silences (starts_at);
CREATE INDEX IF NOT EXISTS silences_end   ON silences (ends_at);
CREATE INDEX IF NOT EXISTS silences_id    ON silences (id());
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
	rows, err := s.db.Query(`
		SELECT id(), matchers, starts_at, ends_at, created_at, created_by, comment
		FROM silences 
		ORDER BY starts_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var silences []*types.Silence

	for rows.Next() {
		var (
			sil      model.Silence
			matchers string
		)

		if err := rows.Scan(&sil.ID, &matchers, &sil.StartsAt, &sil.EndsAt, &sil.CreatedAt, &sil.CreatedBy, &sil.Comment); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(matchers), &sil.Matchers); err != nil {
			return nil, err
		}

		silences = append(silences, types.NewSilence(&sil))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return silences, nil
}

func (s *SQLSilences) Set(sil *types.Silence) (uint64, error) {
	mb, err := json.Marshal(sil.Silence.Matchers)
	if err != nil {
		return 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(`
		INSERT INTO silences(matchers, starts_at, ends_at, created_at, created_by. comment)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
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
	row := s.db.QueryRow(`
		SELECT id(), matchers, starts_at, ends_at, created_at, created_by, comment
		FROM silences
		WHERE id() == $1
	`, sid)

	var (
		sil      model.Silence
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

	return types.NewSilence(&sil), nil
}
