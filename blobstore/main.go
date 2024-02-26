package blobstore

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const keyPrefixFile = "files"

var db *badger.DB
var logger log.Logger

func Init(dir string, lg log.Logger) error {
	if db != nil {
		panic("blobstore already initialized")
	}
	logger = lg
	dir = path.Join(dir, "blobstore")
	var err error
	db, err = badger.Open(badger.DefaultOptions(dir).WithLoggingLevel(badger.ERROR))
	if err != nil {
		return err
	}

	return nil
}

func Close() error {
	return db.Close()
}

type File struct {
	Data               []byte
	ContentType        *string
	ContentDisposition *string
	Secret             *string
}

func checkDb() {
	if db == nil {
		panic("blob storage is not initialized")
	}
}

func PutFileName(bucket, name string, f *File, expired *time.Duration) (string, error) {
	key := strings.Join([]string{bucket, name}, "/")
	return PutFileKey(key, f, expired)
}

func PutFileKey(key string, f *File, expired *time.Duration) (string, error) {
	checkDb()
	fullKey := strings.Join([]string{keyPrefixFile, key}, "/")
	level.Info(logger).Log("msg", "put file to blobstore", "key", key)
	data, err := json.Marshal(&f)
	if err != nil {
		return "", err
	}

	if f.ContentType == nil {
		ct := http.DetectContentType(f.Data)
		f.ContentType = &ct
	}

	err = db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(fullKey), data)
		if expired != nil {
			e.WithTTL(*expired)
		}
		return txn.SetEntry(e)
	})
	if err != nil {
		return "", err
	}

	return key, nil
}

func GetFileName(bucket, name string) (*File, error) {
	key := strings.Join([]string{bucket, name}, "/")
	return GetFileKey(key)
}

func GetFileKey(key string) (*File, error) {
	checkDb()
	f := &File{}
	fullKey := strings.Join([]string{keyPrefixFile, key}, "/")
	err := db.View(func(txn *badger.Txn) error {
		ent, err := txn.Get([]byte(fullKey))
		if err != nil {
			return err
		}
		err = ent.Value(func(val []byte) error {
			return json.Unmarshal(val, f)
		})
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return f, nil
}
