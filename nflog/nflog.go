package nflog

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	pb "github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/weaveworks/mesh"
)

var ErrNotFound = errors.New("not found")

// Log stores and serves information about notifications
// about byte-slice addressed alert objects to different receivers.
type Log interface {
	// The Log* methods store a notification log entry for
	// a fully qualified receiver and a given IDs identifying the
	// alert object.
	LogActive(r *pb.Receiver, key, hash []byte) error
	LogResolved(r *pb.Receiver, key, hash []byte) error

	// Query the log along the given Paramteres.
	//
	// TODO(fabxc):
	// - extend the interface by a `QueryOne` method?
	// - return an iterator rather than a materialized list?
	Query(p ...QueryParam) ([]*pb.Entry, error)
	// Delete log entries along the given Parameters. Returns
	// the number of deleted entries.
	Delete(p ...DeleteParam) (int, error)

	// Snapshot the current log state and return the number
	// of bytes written.
	Snapshot(w io.Writer) (int, error)
}

// query currently allows filtering by and/or receiver group key.
// It is configured via QueryParameter functions.
//
// TODO(fabxc): Future versions could allow querying a certain receiver
// group or a given time interval.
type query struct {
	recv     *pb.Receiver
	groupKey []byte
}

// QueryParam is a function that modifies a query to incorporate
// a set of parameters. Returns an error for invalid or conflicting
// parameters.
type QueryParam func(*query) error

// QReceiver adds a receiver parameter to a query.
func QReceiver(r *pb.Receiver) QueryParam {
	return func(q *query) error {
		q.recv = r
		return nil
	}
}

// QGroupKey adds a group key as querying argument.
func QGroupKey(gk []byte) QueryParam {
	return func(q *query) error {
		q.groupKey = gk
		return nil
	}
}

// delQuery holds parameters for a deletion query.
// TODO(fabxc): can this be consolidated with regular QueryParams?
type delQuery struct {
	// Delete log entries that are expired. Does NOT delete
	// unexpired entries if set to false.
	expired bool
}

// DeleteParam is a function that modifies parameters of a delete request.
// Returns an error for invalid of conflicting parameters.
type DeleteParam func(*delQuery) error

// DExpired adds a parameter to delete expired log entries.
func DExpired() DeleteParam {
	return func(d *delQuery) error {
		d.expired = true
		return nil
	}
}

type nlog struct {
	retention time.Duration
	now       func() time.Time

	mtx sync.RWMutex
	// For now we only store the most recently added log entry.
	// The key is a serialized concatenation of group key and receiver.
	entries map[string]*pb.MeshEntry
}

// Option configures a new Log implementation.
type Option func(*nlog) error

// WithMesh registers the log with a mesh network with which
// the log state will be shared.
func WithMesh(mr *mesh.Router) Option {
	return func(l *nlog) error {
		panic("not implemented")
	}
}

// WithRetention sets the retention time for log entries.
func WithRetention(d time.Duration) Option {
	return func(l *nlog) error {
		l.retention = d
		return nil
	}
}

// WithNow overwrites the function used to retrieve a timestamp
// for the current point in time.
// This is generally useful for injection during tests.
func WithNow(f func() time.Time) Option {
	return func(l *nlog) error {
		l.now = f
		return nil
	}
}

// New creates a new notification log based on the provided options.
// The snapshot is loaded into the Log if it is set.
func New(snapshot io.Reader, opts ...Option) (Log, error) {
	l := &nlog{
		now:     time.Now,
		entries: map[string]*pb.MeshEntry{},
	}
	for _, o := range opts {
		if err := o(l); err != nil {
			return nil, err
		}
	}
	if snapshot != nil {
		if err := l.loadSnapshot(snapshot); err != nil {
			return l, err
		}
	}
	return l, nil
}

// LogActive implements the Log interface.
func (l *nlog) LogActive(r *pb.Receiver, key, hash []byte) error {
	return l.log(r, key, hash, false)
}

// LogResolved implements the Log interface.
func (l *nlog) LogResolved(r *pb.Receiver, key, hash []byte) error {
	return l.log(r, key, hash, true)
}

func (l *nlog) log(r *pb.Receiver, gkey, ghash []byte, resolved bool) error {
	// Write all entries with the same timestamp.
	now := l.now()
	key := fmt.Sprintf("%s:%s", r, gkey)

	l.mtx.Lock()
	defer l.mtx.Unlock()

	if prevle, ok := l.entries[key]; ok {
		// Entry already exists, only overwrite if timestamp is newer.
		// This may with raciness or clock-drift across AM nodes.
		prevts, err := ptypes.Timestamp(prevle.Entry.Timestamp)
		if err != nil {
			return err
		}
		if prevts.After(now) {
			return nil
		}
	}

	ts, err := ptypes.TimestampProto(now)
	if err != nil {
		return err
	}
	expts, err := ptypes.TimestampProto(now.Add(l.retention))
	if err != nil {
		return err
	}

	l.entries[key] = &pb.MeshEntry{
		Entry: &pb.Entry{
			Receiver:  r,
			GroupKey:  gkey,
			GroupHash: ghash,
			Resolved:  resolved,
			Timestamp: ts,
		},
		ExpiresAt: expts,
	}
	return nil
}

// Delete implements the Log interface.
func (l *nlog) Delete(params ...DeleteParam) (int, error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	var del delQuery
	for _, p := range params {
		if err := p(&del); err != nil {
			return 0, err
		}
	}
	if !del.expired {
		return 0, errors.New("only expiration deletion supported")
	}
	now := l.now()
	var n int

	for k, le := range l.entries {
		if ets, err := ptypes.Timestamp(le.ExpiresAt); err != nil {
			return n, err
		} else if ets.Before(now) {
			delete(l.entries, k)
			n++
		}
	}

	return n, nil
}

// Query implements the Log interface.
func (l *nlog) Query(params ...QueryParam) ([]*pb.Entry, error) {
	q := &query{}
	for _, p := range params {
		if err := p(q); err != nil {
			return nil, err
		}
	}
	// TODO(fabxc): For now our only query mode is the most recent entry for a
	// receiver/group_key combination.
	if q.recv == nil || q.groupKey == nil {
		// TODO(fabxc): allow more complex queries in the future.
		// How to enable pagination?
		return nil, errors.New("no query parameters specified")
	}

	l.mtx.RLock()
	defer l.mtx.RUnlock()

	key := fmt.Sprintf("%s,%s", q.recv, q.groupKey)

	if le, ok := l.entries[key]; ok {
		return []*pb.Entry{le.Entry}, nil
	}
	return nil, ErrNotFound
}

func (l *nlog) loadSnapshot(r io.Reader) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	for {
		var e pb.MeshEntry
		if _, err := pbutil.ReadDelimited(r, &e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		key := fmt.Sprintf("%s,%s", r, e.Entry.GroupKey)
		l.entries[key] = &e
	}

	return nil
}

// Snapshot implements the Log interface.
func (l *nlog) Snapshot(w io.Writer) (int, error) {
	l.mtx.RLock()
	defer l.mtx.RUnlock()

	var n int
	for _, e := range l.entries {
		m, err := pbutil.WriteDelimited(w, e)
		if err != nil {
			return n + m, err
		}
		n += m
	}
	return n, nil
}
