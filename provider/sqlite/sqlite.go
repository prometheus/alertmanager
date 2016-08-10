package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

const createAlertsTable = `
CREATE TABLE IF NOT EXISTS alerts (
	id          integer PRIMARY KEY AUTOINCREMENT,
	fingerprint integer,
	labels      blob,
	annotations blob,
	starts_at   timestamp,
	ends_at     timestamp,
	updated_at  timestamp,
	timeout     integer
);
CREATE INDEX IF NOT EXISTS fingerprint    ON alerts (fingerprint);
CREATE INDEX IF NOT EXISTS alerts_start   ON alerts (starts_at);
CREATE INDEX IF NOT EXISTS alerts_end     ON alerts (ends_at);
CREATE INDEX IF NOT EXISTS alerts_updated ON alerts (updated_at);
`

var dbmtx sync.Mutex

type Alerts struct {
	db *sql.DB

	mtx       sync.RWMutex
	listeners map[int]chan *types.Alert
	next      int
	insertCh  chan *types.Alert
}

func NewAlerts(db *sql.DB) (*Alerts, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createAlertsTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	alerts := &Alerts{
		db:        db,
		listeners: map[int]chan *types.Alert{},
		insertCh:  make(chan *types.Alert, 100),
	}
	return alerts, nil
}

// Subscribe implements the Alerts interface.
func (a *Alerts) Subscribe() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)
	alerts, err := a.getPending()

	i := a.next
	a.next++

	a.mtx.Lock()
	a.listeners[i] = ch
	a.mtx.Unlock()

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

	return provider.NewAlertIterator(ch, done, err)
}

// GetPending implements the Alerts interface.
func (a *Alerts) GetPending() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)

	alerts, err := a.getPending()

	go func() {
		defer close(ch)

		for _, a := range alerts {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}
	}()

	return provider.NewAlertIterator(ch, done, err)
}

func (a *Alerts) getPending() ([]*types.Alert, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	// Get the last instance for each alert.
	rows, err := a.db.Query(`
		SELECT a1.labels, a1.annotations, a1.starts_at, a1.ends_at, a1.updated_at, a1.timeout
		FROM alerts AS a1
		LEFT OUTER JOIN alerts AS a2
		  ON a1.fingerprint = a2.fingerprint AND a1.updated_at < a2.updated_at
		WHERE a2.fingerprint IS NULL;
	`)
	if err != nil {
		return nil, err
	}

	var alerts []*types.Alert
	for rows.Next() {
		var (
			labels      []byte
			annotations []byte
			al          types.Alert
		)
		if err := rows.Scan(
			&labels,
			&annotations,
			&al.StartsAt,
			&al.EndsAt,
			&al.UpdatedAt,
			&al.Timeout,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(labels, &al.Labels); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(annotations, &al.Annotations); err != nil {
			return nil, err
		}

		alerts = append(alerts, &al)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return alerts, nil
}

// Get implements the Alerts interface.
func (a *Alerts) Get(model.Fingerprint) (*types.Alert, error) {
	return nil, nil
}

// Put implements the Alerts interface.
func (a *Alerts) Put(alerts ...*types.Alert) error {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}

	// The insert invariant requires that there are no two alerts with the same
	// fingerprint that have overlapping activity range ([StartsAt:EndsAt]).
	// Such alerts are merged into a single one with the union of both intervals
	// as its new activity interval.
	// The exact merge procedure is defined on the Alert structure. Here, we just
	// care about finding intersecting alerts for each new inserts, deleting them
	// if existant, and insert the new alert we retrieved by merging.
	overlap, err := tx.Prepare(`
		SELECT id, annotations, starts_at, ends_at, updated_at, timeout
		FROM alerts
		WHERE fingerprint == $1 AND (
			(starts_at <= $2 AND ends_at >= $2) OR
			(starts_at <= $3 AND ends_at >= $3)
		)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer overlap.Close()

	delOverlap, err := tx.Prepare(`
		DELETE FROM alerts WHERE id IN (
			SELECT id FROM alerts
			WHERE fingerprint == $1 AND (
				(starts_at <= $2 AND ends_at >= $2) OR
				(starts_at <= $3 AND ends_at >= $3)
			)
		)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer delOverlap.Close()

	insert, err := tx.Prepare(`
		INSERT INTO alerts(fingerprint, labels, annotations, starts_at, ends_at, updated_at, timeout)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer insert.Close()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		// Retrieve all intersecting alerts and delete them.
		olaps, err := overlap.Query(int64(fp), alert.StartsAt, alert.EndsAt)
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
				ann []byte
			)
			if err := olaps.Scan(
				&id,
				&ann,
				&na.StartsAt,
				&na.EndsAt,
				&na.UpdatedAt,
				&na.Timeout,
			); err != nil {
				tx.Rollback()
				return err
			}
			if err := json.Unmarshal(ann, &na.Annotations); err != nil {
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
		if _, err := delOverlap.Exec(int64(fp), alert.StartsAt, alert.EndsAt); err != nil {
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
			int64(fp),
			labels,
			annotations,
			alert.StartsAt,
			alert.EndsAt,
			alert.UpdatedAt,
			alert.Timeout,
		)
		if err != nil {
			tx.Rollback()
			return err
		}

		a.mtx.RLock()
		for _, ch := range a.listeners {
			ch <- alert
		}
		a.mtx.RUnlock()
	}

	tx.Commit()

	return nil
}

const createNotifyInfoTable = `
CREATE TABLE IF NOT EXISTS notify_info (
	alert      bigint,
	receiver   text,
	resolved   integer,
	timestamp  timestamp
);
CREATE INDEX IF NOT EXISTS notify_done ON notify_info (resolved);
CREATE UNIQUE INDEX IF NOT EXISTS alert_receiver ON notify_info (alert,receiver);
`

type Notifies struct {
	db *sql.DB
}

func NewNotifies(db *sql.DB) (*Notifies, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createNotifyInfoTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &Notifies{db: db}, nil
}

// Get implements the Notifies interface.
func (n *Notifies) Get(dest string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	var result []*types.NotifyInfo

	for _, fp := range fps {
		row := n.db.QueryRow(`
			SELECT alert, receiver, resolved, timestamp
			FROM notify_info
			WHERE receiver == $1 AND alert == $2
		`, dest, int64(fp))

		var alertFP int64

		var ni types.NotifyInfo
		err := row.Scan(
			&alertFP,
			&ni.Receiver,
			&ni.Resolved,
			&ni.Timestamp,
		)
		if err == sql.ErrNoRows {
			result = append(result, nil)
			continue
		}
		if err != nil {
			return nil, err
		}

		ni.Alert = model.Fingerprint(alertFP)

		result = append(result, &ni)
	}

	return result, nil
}

// Set implements the Notifies interface.
func (n *Notifies) Set(ns ...*types.NotifyInfo) error {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := n.db.Begin()
	if err != nil {
		return err
	}

	insert, err := tx.Prepare(`
		INSERT INTO notify_info(alert, receiver, resolved, timestamp)
		VALUES ($1, $2, $3, $4);
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer insert.Close()

	del, err := tx.Prepare(`
		DELETE FROM notify_info
		WHERE alert == $1 AND receiver == $2
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer del.Close()

	for _, ni := range ns {
		if _, err := del.Exec(int64(ni.Alert), ni.Receiver); err != nil {
			tx.Rollback()
			return fmt.Errorf("deleting old notify failed: %s", err)
		}
		if _, err := insert.Exec(
			int64(ni.Alert),
			ni.Receiver,
			ni.Resolved,
			ni.Timestamp,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("inserting new notify failed: %s", err)
		}
	}

	tx.Commit()

	return nil
}

const createSilencesTable = `
CREATE TABLE IF NOT EXISTS silences (
	id         integer PRIMARY KEY AUTOINCREMENT,
	matchers   blob,
	starts_at  timestamp,
	ends_at    timestamp,
	created_at timestamp,
	created_by text,
	comment    text
);
CREATE INDEX IF NOT EXISTS silences_start ON silences (starts_at);
CREATE INDEX IF NOT EXISTS silences_end   ON silences (ends_at);
`

type Silences struct {
	db     *sql.DB
	marker types.Marker
}

// NewSilences returns a new Silences based on the provided SQL DB.
func NewSilences(db *sql.DB, mk types.Marker) (*Silences, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createSilencesTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &Silences{db: db, marker: mk}, nil
}

// Mutes implements the Muter interface.
func (s *Silences) Mutes(lset model.LabelSet) bool {
	sils, err := s.All()
	if err != nil {
		log.Errorf("retrieving silences failed: %s", err)
		// In doubt, do not silence anything.
		return false
	}

	for _, sil := range sils {
		if sil.Mutes(lset) {
			s.marker.SetSilenced(lset.Fingerprint(), sil.ID)
			return true
		}
	}

	s.marker.SetSilenced(lset.Fingerprint())
	return false
}

var ErrorNoMoreSilences = errors.New("fewer silences found than requested")

// LimitGet implements the Silences interface.
func (s *Silences) Query(n uint64, o uint64) ([]*types.Silence, error) {
	return nil, nil
}

// All implements the Silences interface.
func (s *Silences) All() ([]*types.Silence, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	rows, err := s.db.Query(`
		SELECT id, matchers, starts_at, ends_at, created_at, created_by, comment
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
			matchers []byte
		)

		if err := rows.Scan(
			&sil.ID,
			&matchers,
			&sil.StartsAt,
			&sil.EndsAt,
			&sil.CreatedAt,
			&sil.CreatedBy,
			&sil.Comment,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(matchers, &sil.Matchers); err != nil {
			return nil, err
		}

		silences = append(silences, types.NewSilence(&sil))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return silences, nil
}

// Set impelements the Silences interface.
func (s *Silences) Set(sil *types.Silence) (uint64, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	mb, err := json.Marshal(sil.Silence.Matchers)
	if err != nil {
		return 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(`
		INSERT INTO silences(matchers, starts_at, ends_at, created_at, created_by, comment)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
		mb,
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

// Del implements the Silences interface.
func (s *Silences) Del(sid uint64) error {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM silences WHERE id == $1`, sid); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()

	return nil
}

// Get implements the Silences interface.
func (s *Silences) Get(sid uint64) (*types.Silence, error) {
	dbmtx.Lock()
	defer dbmtx.Unlock()

	row := s.db.QueryRow(`
		SELECT id, matchers, starts_at, ends_at, created_at, created_by, comment
		FROM silences
		WHERE id == $1
	`, sid)

	var (
		sil      model.Silence
		matchers []byte
	)
	err := row.Scan(
		&sil.ID,
		&matchers,
		&sil.StartsAt,
		&sil.EndsAt,
		&sil.CreatedAt,
		&sil.CreatedBy,
		&sil.Comment,
	)
	if err == sql.ErrNoRows {
		return nil, provider.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(matchers, &sil.Matchers); err != nil {
		return nil, err
	}

	return types.NewSilence(&sil), nil
}
