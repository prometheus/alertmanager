// Copyright 2015 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cznic/ql"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

func init() {
	ql.RegisterDriver()
}

// SQLNotifies implements a Notifies provider based on a SQL DB.
type SQLNotifies struct {
	db *sql.DB
}

// NewSQLNotifies returns a new SQLNotifies based on the provided SQL DB.
func NewSQLNotifies(db *sql.DB) (*SQLNotifies, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createNotifyInfoTable); err != nil {
		tx.Rollback()
		return nil, err
	}

	// XXX(bug-ish): The selection of pending alerts uses a NOT IN clause
	// that will falsely be evaluated to false if the nested SELECT statement
	// has no results at all.
	// Thus, we insert a fake alert so there is always at least one result.
	// The fingerprint must not be NULL as it doesn't work.
	row := tx.QueryRow(`SELECT count() FROM notify_info WHERE alert == 0`)

	var count int
	if err := row.Scan(&count); err != nil {
		tx.Rollback()
		return nil, err
	}
	if count == 0 {
		_, err := tx.Exec(`
			INSERT INTO notify_info(alert, resolved)
			VALUES (0, true)
		`)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	tx.Commit()

	return &SQLNotifies{db: db}, nil
}

const createNotifyInfoTable = `
CREATE TABLE IF NOT EXISTS notify_info (
	alert     int64,
	receiver  string,
	resolved  bool,
	timestamp time
);
CREATE UNIQUE INDEX IF NOT EXISTS notify_alert
	ON notify_info (alert, receiver);
CREATE INDEX IF NOT EXISTS notify_done
	ON notify_info (resolved);
`

// Get implements the Notifies interface.
func (n *SQLNotifies) Get(dest string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
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
func (n *SQLNotifies) Set(ns ...*types.NotifyInfo) error {
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

// SQLAlerts implements an Alerts provider based on a SQL DB.
type SQLAlerts struct {
	db *sql.DB

	mtx       sync.RWMutex
	listeners map[int]chan *types.Alert
	next      int
}

// NewSQLAlerts returns a new SQLAlerts based on the provided SQL DB.
func NewSQLAlerts(db *sql.DB) (*SQLAlerts, error) {
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
	fingerprint int64,
	labels      string,
	annotations string,
	starts_at   time,
	ends_at     time,
	updated_at  time,
	timeout     bool
);
CREATE INDEX IF NOT EXISTS alerts_start ON alerts (starts_at);
CREATE INDEX IF NOT EXISTS alerts_end   ON alerts (ends_at);
`

// Subscribe implements the Alerts interface.
func (a *SQLAlerts) Subscribe() AlertIterator {
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

	return alertIterator{
		ch:   ch,
		done: done,
		err:  err,
	}
}

// GetPending implements the Alerts interface.
func (a *SQLAlerts) GetPending() AlertIterator {
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

	return alertIterator{
		ch:   ch,
		done: done,
		err:  err,
	}
}

func (a *SQLAlerts) getPending() ([]*types.Alert, error) {
	// TODO(fabxc): hacky query but proper one doesn't work for some reason.
	// A few too many hacks – consider another DB in the long run.
	rows, err := a.db.Query(`
		SELECT labels, annotations, starts_at, ends_at, updated_at, timeout
		FROM alerts
		WHERE
			fingerprint NOT IN (
				SELECT alert FROM notify_info WHERE resolved
			)
		ORDER BY starts_at
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

// Get implements the Alerts interface.
func (a *SQLAlerts) Get(model.Fingerprint) (*types.Alert, error) {
	return nil, nil
}

// Put implements the Alerts interface.
func (a *SQLAlerts) Put(alerts ...*types.Alert) error {
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
		SELECT id(), annotations, starts_at, ends_at, updated_at, timeout
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
		DELETE FROM alerts WHERE id() IN (
			SELECT id() FROM alerts
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
				ann string
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

		a.mtx.RLock()
		for _, ch := range a.listeners {
			ch <- alert
		}
		a.mtx.RUnlock()
	}

	tx.Commit()

	return nil
}

// SQLSilences implements a Silences provider based on a SQL DB.
type SQLSilences struct {
	db     *sql.DB
	marker types.Marker
}

// NewSQLSilences returns a new SQLSilences based on the provided SQL DB.
func NewSQLSilences(db *sql.DB, mk types.Marker) (*SQLSilences, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(createSilencesTable); err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &SQLSilences{db: db, marker: mk}, nil
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

// Mutes implements the Muter interface.
func (s *SQLSilences) Mutes(lset model.LabelSet) bool {
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

// All implements the Silences interface.
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

// Set impelements the Silences interface.
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
		INSERT INTO silences(matchers, starts_at, ends_at, created_at, created_by, comment)
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

// Del implements the Silences interface.
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

// Get implements the Silences interface.
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
