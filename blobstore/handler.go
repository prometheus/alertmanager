package blobstore

import (
	"crypto/subtle"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

func GetHandler(logger log.Logger) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		key := req.PathValue("key")
		file, err := GetFileKey(key)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			level.Error(logger).Log("msg", "unable to read file from blobstore", "key", key, "err", err)
			return
		}

		if file.Secret != nil {
			s := req.URL.Query().Get("s")
			if subtle.ConstantTimeCompare([]byte(*file.Secret), []byte(s)) == 0 {
				w.WriteHeader(http.StatusForbidden)
			}
		}

		if file.ContentType != nil {
			w.Header().Add("Content-Type", *file.ContentType)
		}
		if file.ContentDisposition != nil {
			w.Header().Add("Content-Disposition", *file.ContentType)
		}
		w.Write(file.Data)
		return
	}
}
