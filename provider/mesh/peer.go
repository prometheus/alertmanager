package mesh

import (
	"fmt"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/weaveworks/mesh"
)

type NotificationInfos struct {
	st     *notificationState
	send   mesh.Gossip
	logger log.Logger
}

func NewNotificationInfos(logger log.Logger) *NotificationInfos {
	return &NotificationInfos{
		logger: logger,
		st:     newNotificationState(),
	}
}

func (ni *NotificationInfos) Register(g mesh.Gossip) {
	ni.send = g
}

func (ni *NotificationInfos) Run(retention time.Duration) {
	ni.st.run(retention)
}

func (ni *NotificationInfos) Stop() {
	ni.st.stop()
}

func (ni *NotificationInfos) Gossip() mesh.GossipData {
	return ni.st.copy()
}

func (ni *NotificationInfos) OnGossip(b []byte) (mesh.GossipData, error) {
	set, err := decodeNotificationSet(b)
	if err != nil {
		return nil, err
	}
	d := ni.st.mergeDelta(set)
	// The delta is newly created and we are the only one holding it so far.
	// Thus, we can access without locking.
	if len(d.set) == 0 {
		return nil, nil // per OnGossip contract
	}
	return d, nil
}

func (ni *NotificationInfos) OnGossipBroadcast(_ mesh.PeerName, b []byte) (mesh.GossipData, error) {
	set, err := decodeNotificationSet(b)
	if err != nil {
		return nil, err
	}
	return ni.st.mergeDelta(set), nil
}

func (ni *NotificationInfos) OnGossipUnicast(_ mesh.PeerName, b []byte) error {
	set, err := decodeNotificationSet(b)
	if err != nil {
		return err
	}
	ni.st.mergeComplete(set)
	return nil
}

func (ni *NotificationInfos) Set(ns ...*types.NotifyInfo) error {
	set := map[string]notificationEntry{}
	for _, n := range ns {
		k := fmt.Sprintf("%s:%s", n.Alert, n.Receiver)
		set[k] = notificationEntry{
			Resolved:  n.Resolved,
			Timestamp: n.Timestamp,
		}
	}
	update := &notificationState{set: set}

	ni.st.Merge(update)
	ni.send.GossipBroadcast(update)
	return nil
}

func (ni *NotificationInfos) Get(recv string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	res := make([]*types.NotifyInfo, 0, len(fps))
	for _, fp := range fps {
		k := fmt.Sprintf("%s:%s", fp, recv)
		if e, ok := ni.st.set[k]; ok {
			res = append(res, &types.NotifyInfo{
				Alert:     fp,
				Receiver:  recv,
				Resolved:  e.Resolved,
				Timestamp: e.Timestamp,
			})
		} else {
			res = append(res, nil)
		}
	}
	return res, nil
}

type Silences struct {
	st     *silenceState
	mk     types.Marker
	send   mesh.Gossip
	logger log.Logger
}

func NewSilences(mk types.Marker, logger log.Logger) *Silences {
	return &Silences{
		st:     newSilenceState(),
		mk:     mk,
		logger: logger,
	}
}

func (s *Silences) Register(g mesh.Gossip) {
	s.send = g
}

func (s *Silences) Run(retention time.Duration) {
	s.st.run(retention)
}

func (s *Silences) Stop() {
	s.st.stop()
}

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

func (s *Silences) All() ([]*types.Silence, error) {
	s.st.mtx.RLock()
	defer s.st.mtx.RUnlock()
	res := make([]*types.Silence, 0, len(s.st.m))

	for _, sil := range s.st.m {
		res = append(res, sil)
	}
	return res, nil
}

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

func (s *Silences) Get(id uuid.UUID) (*types.Silence, error) {
	s.st.mtx.RLock()
	defer s.st.mtx.RUnlock()

	sil, ok := s.st.m[id]
	if !ok {
		return nil, provider.ErrNotFound
	}
	// TODO(fabxc): ensure that silence objects are never modified; just replaced.
	return sil, nil
}

func (s *Silences) Gossip() mesh.GossipData {
	return s.st.copy()
}

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

func (s *Silences) OnGossipBroadcast(_ mesh.PeerName, b []byte) (mesh.GossipData, error) {
	set, err := decodeSilenceSet(b)
	if err != nil {
		return nil, err
	}
	d := s.st.mergeDelta(set)
	return d, nil
}

func (s *Silences) OnGossipUnicast(_ mesh.PeerName, b []byte) error {
	set, err := decodeSilenceSet(b)
	if err != nil {
		return err
	}
	s.st.mergeComplete(set)
	return nil
}
