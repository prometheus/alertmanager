// Copyright The Prometheus Authors
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

package notify

import (
	"fmt"
	"sync"

	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// MetadataStore is a temporary in-memory store for notification metadata
// (e.g., Slack message_ts, Jira issue keys) until protobuf Entry supports metadata field.
// This allows integrations to update existing notifications instead of creating new ones.
type MetadataStore struct {
	mtx  sync.RWMutex
	data map[string]map[string]string // key: stateKey(groupKey, receiver) -> metadata map
}

// NewMetadataStore creates a new MetadataStore.
func NewMetadataStore() *MetadataStore {
	return &MetadataStore{
		data: make(map[string]map[string]string),
	}
}

// Set stores metadata for a given receiver and group key.
func (s *MetadataStore) Set(receiver *nflogpb.Receiver, groupKey string, metadata map[string]string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	key := stateKey(groupKey, receiver)
	s.data[key] = metadata
}

// Get retrieves metadata for a given receiver and group key.
func (s *MetadataStore) Get(receiver *nflogpb.Receiver, groupKey string) (map[string]string, bool) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	key := stateKey(groupKey, receiver)
	metadata, ok := s.data[key]
	return metadata, ok
}

// Delete removes metadata for a given receiver and group key.
func (s *MetadataStore) Delete(receiver *nflogpb.Receiver, groupKey string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	key := stateKey(groupKey, receiver)
	delete(s.data, key)
}

// stateKey returns a string key for a log entry consisting of the group key and receiver.
// This matches the key generation in nflog.
func stateKey(gkey string, r *nflogpb.Receiver) string {
	return receiverKey(gkey, r)
}

// receiverKey creates a unique key from group key and receiver.
// Format matches nflog's internal stateKey format: "groupKey:groupName/integration/idx".
func receiverKey(groupKey string, r *nflogpb.Receiver) string {
	return groupKey + ":" + receiverString(r)
}

// receiverString returns a string representation of the receiver.
func receiverString(r *nflogpb.Receiver) string {
	return fmt.Sprintf("%s/%s/%d", r.GroupName, r.Integration, r.Idx)
}
