package crdt

import (
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("not found")
)

type Elem struct {
	Key   string
	Score uint64
	Value interface{}
}

type Set interface {
	Get(key string) (*Elem, error)
	List() ([]*Elem, error)

	Add(key string, score uint64, v interface{}) error
	Del(key string, score uint64) error
}

type Entity struct {
	add uint64
	del uint64
	val interface{}
}

type LWW struct {
	storage Storage
}

func NewLWW(storage Storage) Set {
	return &LWW{
		storage: storage,
	}
}

func (lww *LWW) Add(key string, score uint64, val interface{}) error {
	e, err := lww.storage.Get(key)
	if err != nil && err != ErrNotFound {
		return err
	}
	if err == ErrNotFound {
		e = &Entity{0, 0, nil}
	}

	if e.add > score || e.del > score {
		return nil
	}

	e.del = 0
	e.add = score
	e.val = val

	return lww.storage.Set(key, e)
}

func (lww *LWW) Del(key string, score uint64) error {
	e, err := lww.storage.Get(key)
	if err != nil {
		return err
	}

	if e.add > score || e.del > score {
		return nil
	}

	e.del = score
	e.add = 0
	e.val = nil

	return lww.storage.Set(key, e)
}

func (lww *LWW) Get(key string) (*Elem, error) {
	e, err := lww.storage.Get(key)
	if err != nil {
		return nil, err
	}
	if e.del > e.add {
		return nil, ErrNotFound
	}

	return &Elem{
		Key:   key,
		Score: e.add,
		Value: e.val,
	}, nil
}

func (lww *LWW) List() ([]*Elem, error) {
	kval, err := lww.storage.All()
	if err != nil {
		return nil, err
	}

	var res []*Elem
	for k, e := range kval {
		if e.add <= e.del {
			continue
		}
		res = append(res, &Elem{
			Key:   k,
			Score: e.add,
			Value: e.val,
		})
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
