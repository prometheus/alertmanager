package mesh

import (
	"fmt"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
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
		k := fmt.Sprintf("%x:%s", n.Alert, n.Receiver)
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

func (ni *NotificationInfos) Get(dest string, fps ...model.Fingerprint) ([]*types.NotifyInfo, error) {
	res := make([]*types.NotifyInfo, 0, len(fps))
	for _, fp := range fps {
		k := fmt.Sprintf("%x:%s", fp, dest)
		if e, ok := ni.st.set[k]; ok {
			res = append(res, &types.NotifyInfo{
				Alert:     fp,
				Receiver:  dest,
				Resolved:  e.Resolved,
				Timestamp: e.Timestamp,
			})
		} else {
			res = append(res, nil)
		}
	}
	return res, nil
}
