package twilio

import (
	"sync"
	"time"

	"github.com/gofrs/uuid"
)

var Storage = NewStorage()

type Entity struct {
	Data      []byte
	CreatedAt time.Time
}

type storage struct {
	store    map[string]Entity
	mutex    sync.RWMutex
	Lifetime time.Duration
}

func (st *storage) cleaner() {
	for range time.Tick(time.Minute * 10) {
		st.mutex.Lock()
		for k, v := range st.store {
			if v.CreatedAt.Add(st.Lifetime).Before(time.Now()) {
				delete(st.store, k)
			}
		}
		st.mutex.Unlock()
	}
}

func (st *storage) Get(id string) []byte {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	if val, ok := st.store[id]; !ok {
		return nil
	} else {
		return val.Data
	}
}

func (st *storage) Put(data []byte) string {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	id, _ := uuid.NewV1()
	st.store[id.String()] = Entity{Data: data, CreatedAt: time.Now()}
	return id.String()
}

func NewStorage() *storage {
	st := &storage{store: make(map[string]Entity), mutex: sync.RWMutex{}, Lifetime: time.Hour * 24}
	go st.cleaner()
	return st
}
