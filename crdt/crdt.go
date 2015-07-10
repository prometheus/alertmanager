package crdt

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	ErrNotFound = errors.New("not found")
)

type Elem struct {
	Key   string
	Score uint64
	Value json.RawMessage
}

type Set interface {
	Get(key string) (*Elem, error)
	List() ([]*Elem, error)

	Add(key string, score uint64, v json.RawMessage) error
	Del(key string, score uint64) error
}

type Entity struct {
	Add uint64
	Del uint64
	Val json.RawMessage
}

type LWW struct {
	storage Storage
}

func NewLWW(storage Storage) Set {
	return &LWW{
		storage: storage,
	}
}

func (lww *LWW) Add(key string, score uint64, val json.RawMessage) error {
	has, err := lww.storage.Has(key)
	if err != nil {
		return err
	}
	var e *Entity

	if !has {
		e = &Entity{0, 0, nil}
	} else {
		e, err = lww.storage.Get(key)
		if err != nil {
			return err
		}
	}

	if e.Add > score || e.Del > score {
		return nil
	}

	e.Del = 0
	e.Add = score
	e.Val = val

	return lww.storage.Set(key, e)
}

func (lww *LWW) Del(key string, score uint64) error {
	e, err := lww.storage.Get(key)
	if err != nil {
		return err
	}

	if e.Add > score || e.Del > score {
		return nil
	}

	e.Del = score
	e.Add = 0
	e.Val = nil

	return lww.storage.Set(key, e)
}

func (lww *LWW) Get(key string) (*Elem, error) {
	e, err := lww.storage.Get(key)
	if err != nil {
		return nil, err
	}
	if e.Del > e.Add {
		return nil, ErrNotFound
	}

	res := &Elem{
		Key:   key,
		Score: e.Add,
		Value: e.Val,
	}

	return res, nil
}

func (lww *LWW) List() ([]*Elem, error) {
	kval, err := lww.storage.All()
	if err != nil {
		return nil, err
	}

	var res []*Elem
	for k, e := range kval {
		if e.Add <= e.Del {
			continue
		}
		el := &Elem{Key: k, Score: e.Add}

		if err := json.Unmarshal(e.Val, &el.Value); err != nil {
			return nil, err
		}

		res = append(res, el)
	}

	return res, nil
}

// Storage is an interface that holds values associated with
// a key. This can be used to connect different storage backends
// to a key set.
type Storage interface {
	Set(key string, e *Entity) error
	Del(key string) error
	Has(key string) (bool, error)
	Get(key string) (*Entity, error)
	All() (map[string]*Entity, error)
}

func NewMemStorage() Storage {
	return &memStorage{
		kval: map[string]*Entity{},
	}
}

type memStorage struct {
	kval map[string]*Entity
	mtx  sync.RWMutex
}

func (s *memStorage) Set(key string, e *Entity) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.kval[key] = e
	return nil
}

func (s *memStorage) Get(key string) (*Entity, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if v, ok := s.kval[key]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (s *memStorage) Del(key string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.kval[key]; !ok {
		return ErrNotFound
	}

	delete(s.kval, key)
	return nil
}

func (s *memStorage) Has(key string) (bool, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	_, ok := s.kval[key]
	return ok, nil
}

func (s *memStorage) All() (map[string]*Entity, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	res := make(map[string]*Entity, len(s.kval))
	for k, v := range s.kval {
		res[k] = v
	}

	return res, nil
}

type ldbStorage struct {
	db *leveldb.DB
}

func NewLevelDBStorage(path string) (Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}

	ldbs := &ldbStorage{
		db: db,
	}
	return ldbs, nil
}

func (s *ldbStorage) Set(key string, e *Entity) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.db.Put([]byte(key), b, nil)
}

func (s *ldbStorage) Get(key string) (*Entity, error) {
	b, err := s.db.Get([]byte(key), nil)
	if err != nil {
		return nil, err
	}

	var e Entity
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}

	return &e, nil
}

func (s *ldbStorage) Del(key string) error {
	return nil
}

func (s *ldbStorage) Has(key string) (bool, error) {
	return s.db.Has([]byte(key), nil)
}

func (s *ldbStorage) All() (map[string]*Entity, error) {
	it := s.db.NewIterator(nil, nil)

	res := map[string]*Entity{}

	for it.Next() {
		var e Entity
		if err := json.Unmarshal(it.Value(), &e); err != nil {
			return nil, err
		}
		res[string(it.Key())] = &e
	}

	return res, nil
}
