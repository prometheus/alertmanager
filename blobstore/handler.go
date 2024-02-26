package blobstore

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/route"
)

func Handler(w http.ResponseWriter, req *http.Request) {
	key := strings.Trim(route.Param(req.Context(), "key"), "/")
	file, err := GetFileKey(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		level.Error(logger).Log("msg", "unable to read file from blobstore", "key", key, "err", err)
		return
	}
	if file == nil {
		level.Warn(logger).Log("msg", "unable to find file in blobstore", "key", key)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if file.Secret != nil {
		s := req.URL.Query().Get("s")
		if subtle.ConstantTimeCompare([]byte(*file.Secret), []byte(s)) == 0 {
			w.WriteHeader(http.StatusForbidden)
			return
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
