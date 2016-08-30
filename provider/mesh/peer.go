package mesh

import (
	"fmt"
	"os"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/weaveworks/mesh"
)

// replaceFile wraps a file that is moved to another filename on closing.
type replaceFile struct {
	*os.File
	filename string
}

func (f *replaceFile) Close() error {
	if err := f.File.Sync(); err != nil {
		return err
	}
	if err := f.File.Close(); err != nil {
		return err
	}
	return os.Rename(f.File.Name(), f.filename)
}

// openReplace opens a new temporary file that is moved to filename on closing.
func openReplace(filename string) (*replaceFile, error) {
	tmpFilename := fmt.Sprintf("%s.%s", filename, utcNow().Format(time.RFC3339Nano))

	f, err := os.Create(tmpFilename)
	if err != nil {
		return nil, err
	}

	rf := &replaceFile{
		File:     f,
		filename: filename,
	}
	return rf, nil
}

// TODO(fabxc): consider making this a flag.
const maintenanceInterval = 15 * time.Minute

type Silences struct {
	st        *silenceState
	mk        types.Marker
	send      mesh.Gossip
	stopc     chan struct{}
	logger    log.Logger
	retention time.Duration
	snapfile  string
}

// NewSilences creates a new Silences object.
func NewSilences(mk types.Marker, logger log.Logger, retention time.Duration, snapfile string) (*Silences, error) {
	s := &Silences{
		st:        newSilenceState(),
		mk:        mk,
		stopc:     make(chan struct{}),
		logger:    logger,
		retention: retention,
		snapfile:  snapfile,
	}
	f, err := os.Open(snapfile)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return s, s.st.loadSnapshot(f)
}

// Register a gossip channel over which silences are shared.
func (s *Silences) Register(g mesh.Gossip) {
	s.send = g
}

// Run blocking background processing. Cannot be run more than once.
func (s *Silences) Run() {
	for {
		select {
		case <-s.stopc:
			return
		case <-time.After(maintenanceInterval):
			s.st.gc(s.retention)
			if err := s.snapshot(); err != nil {
				s.logger.With("err", err).Errorf("Snapshotting failed")
			}
		}
	}
}

func (s *Silences) snapshot() error {
	s.logger.Warnf("creating snapshot")
	f, err := openReplace(s.snapfile)
	if err != nil {
		return err
	}
	if err := s.st.snapshot(f); err != nil {
		return err
	}
	s.logger.Warnf("snapshot created")
	return f.Close()
}

// Stop signals the background processing to be stopped.
func (s *Silences) Stop() {
	log.Errorf("stopping silences")
	close(s.stopc)
	if err := s.snapshot(); err != nil {
		s.logger.With("err", err).Errorf("Snapshotting failed")
	}
}

// Mutes returns true iff any of the known silences mutes the provided label set.
func (s *Silences) Mutes(lset model.LabelSet) bool {
	s.st.mtx.RLock()
	defer s.st.mtx.RUnlock()

	for _, sil := range s.st.m {
		if sil.Mutes(lset) {
			s.mk.SetSilenced(lset.Fingerprint(), sil.ID)
			return true
		}
	}

	s.mk.SetSilenced(lset.Fingerprint())
	return false
}

// All returns a list of all known silences.
func (s *Silences) All() ([]*types.Silence, error) {
	s.st.mtx.RLock()
	defer s.st.mtx.RUnlock()
	res := make([]*types.Silence, 0, len(s.st.m))

	for _, sil := range s.st.m {
		if !sil.Deleted() {
			res = append(res, sil)
		}
	}
	return res, nil
}

// Set overwrites the given silence or creates a new one if it doesn't exist yet.
// The new information is spread via the registered gossip channel.
func (s *Silences) Set(sil *types.Silence) (uuid.UUID, error) {
	if sil.ID == uuid.Nil {
		sil.ID = uuid.NewV4()
	}
	if err := s.st.set(sil); err != nil {
		return uuid.Nil, err
	}

	s.send.GossipBroadcast(&silenceState{
		m: map[uuid.UUID]*types.Silence{
			sil.ID: sil,
		},
	})

	return sil.ID, nil
}

// Del removes the silence with the given ID. The new information is spread via
// the registered gossip channel.
// Active silences are not deleted but their end time is set to now.
//
// TODO(fabxc): consider actually deleting silences that haven't started yet.
func (s *Silences) Del(id uuid.UUID) error {
	sil, err := s.st.del(id)
	if err != nil {
		return err
	}

	update := &silenceState{
		m: map[uuid.UUID]*types.Silence{
			sil.ID: sil,
		},
	}
	s.send.GossipBroadcast(update)

	return nil
}

// Get the silence with the given ID.
func (s *Silences) Get(id uuid.UUID) (*types.Silence, error) {
	s.st.mtx.RLock()
	defer s.st.mtx.RUnlock()

	sil, ok := s.st.m[id]
	if !ok || sil.Deleted() {
		return nil, provider.ErrNotFound
	}
	// TODO(fabxc): ensure that silence objects are never modified; just replaced.
	return sil, nil
}

// Gossip implements the mesh.Gossiper interface.
func (s *Silences) Gossip() mesh.GossipData {
	return s.st.copy()
}

// OnGossip implements the mesh.Gossiper interface.
func (s *Silences) OnGossip(b []byte) (mesh.GossipData, error) {
	set, err := decodeSilenceSet(b)
	if err != nil {
		return nil, err
	}
	d := s.st.mergeDelta(set)
	// The delta is newly created and we are the only one holding it so far.
	// Thus, we can access without locking.
	if len(d.m) == 0 {
		return nil, nil // per OnGossip contract
	}
	return d, nil
}

// OnGossipBroadcast implements the mesh.Gossiper interface.
func (s *Silences) OnGossipBroadcast(_ mesh.PeerName, b []byte) (mesh.GossipData, error) {
	set, err := decodeSilenceSet(b)
	if err != nil {
		return nil, err
	}
	d := s.st.mergeDelta(set)
	return d, nil
}

// OnGossipUnicast implements the mesh.Gossiper interface.
func (s *Silences) OnGossipUnicast(_ mesh.PeerName, b []byte) error {
	set, err := decodeSilenceSet(b)
	if err != nil {
		return err
	}
	s.st.mergeComplete(set)
	return nil
}
