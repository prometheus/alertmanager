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

package apiconnect

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
)

const pageTokenVersion = 1

var (
	errMalformedPageToken = errors.New("malformed page token")
	errExpiredPageToken   = errors.New("expired page token")
)

type pageTokenPayload struct {
	Version      int    `json:"v"`
	Resource     string `json:"r"`
	Cursor       string `json:"c"`
	FilterDigest string `json:"f"`
	ExpiresAt    int64  `json:"e"`
}

type pageTokenCodec struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func newPageTokenCodec(key []byte, ttl time.Duration) *pageTokenCodec {
	return &pageTokenCodec{key: append([]byte(nil), key...), ttl: ttl, now: time.Now}
}

func (codec *pageTokenCodec) encode(resource, cursor, digest string) (string, error) {
	if len(codec.key) == 0 || codec.ttl <= 0 || resource == "" || cursor == "" || digest == "" {
		return "", errors.New("invalid page token codec input")
	}
	payload, err := json.Marshal(pageTokenPayload{
		Version:      pageTokenVersion,
		Resource:     resource,
		Cursor:       cursor,
		FilterDigest: digest,
		ExpiresAt:    codec.now().Add(codec.ttl).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal page token: %w", err)
	}
	signature := codec.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (codec *pageTokenCodec) decode(token, resource, digest string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(codec.key) == 0 {
		return "", errMalformedPageToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errMalformedPageToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, codec.sign(payload)) {
		return "", errMalformedPageToken
	}

	var decoded pageTokenPayload
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return "", errMalformedPageToken
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", errMalformedPageToken
	}
	if decoded.Version != pageTokenVersion || decoded.Resource == "" || decoded.Cursor == "" || decoded.FilterDigest == "" {
		return "", errMalformedPageToken
	}
	if decoded.Resource != resource {
		return "", fmt.Errorf("%w: resource type does not match", errMalformedPageToken)
	}
	if decoded.FilterDigest != digest {
		return "", fmt.Errorf("%w: filters do not match", errMalformedPageToken)
	}
	if !codec.now().Before(time.Unix(decoded.ExpiresAt, 0)) {
		return "", errExpiredPageToken
	}
	return decoded.Cursor, nil
}

func (codec *pageTokenCodec) sign(payload []byte) []byte {
	hash := hmac.New(sha256.New, codec.key)
	_, _ = hash.Write(payload)
	return hash.Sum(nil)
}

func protoFilterDigest(filters ...proto.Message) (string, error) {
	hash := sha256.New()
	var size [8]byte
	for _, filter := range filters {
		if filter == nil {
			binary.BigEndian.PutUint64(size[:], 0)
			_, _ = hash.Write(size[:])
			continue
		}
		data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(filter)
		if err != nil {
			return "", fmt.Errorf("marshal filter: %w", err)
		}
		binary.BigEndian.PutUint64(size[:], uint64(len(data))+1)
		_, _ = hash.Write(size[:])
		_, _ = hash.Write(data)
	}
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil)), nil
}

type paginationSpec struct {
	resource     string
	defaultSize  int
	maximumSize  int
	filterDigest string
}

func paginate[T any](items []T, request *commonv3alpha.PageRequest, spec paginationSpec, codec *pageTokenCodec, cursor func(T) string) ([]T, *commonv3alpha.PageResponse, error) {
	if spec.resource == "" || spec.defaultSize <= 0 || spec.maximumSize < spec.defaultSize || spec.filterDigest == "" || codec == nil {
		return nil, nil, newRPCError(connect.CodeInternal, "invalid pagination configuration", nil)
	}

	pageSize := spec.defaultSize
	var token string
	if request != nil {
		if request.GetPageSize() < 0 {
			return nil, nil, invalidArgumentError(
				"page size must not be negative",
				&commonv3alpha.FieldViolation{Field: "page.page_size", Description: "must not be negative"},
			)
		}
		if request.GetPageSize() > 0 {
			pageSize = min(int(request.GetPageSize()), spec.maximumSize)
		}
		token = request.GetPageToken()
	}

	cursors := make([]string, len(items))
	for index, item := range items {
		cursors[index] = cursor(item)
		if cursors[index] == "" || index > 0 && cursors[index] <= cursors[index-1] {
			return nil, nil, newRPCError(connect.CodeInternal, "resources are not in a unique deterministic order", nil)
		}
	}

	start := 0
	if token != "" {
		previous, err := codec.decode(token, spec.resource, spec.filterDigest)
		if err != nil {
			return nil, nil, invalidArgumentError(
				"invalid page token",
				&commonv3alpha.FieldViolation{Field: "page.page_token", Description: err.Error()},
			)
		}
		start = sort.Search(len(cursors), func(index int) bool { return cursors[index] > previous })
	}

	end := min(start+pageSize, len(items))
	response := &commonv3alpha.PageResponse{}
	if end < len(items) {
		next, err := codec.encode(spec.resource, cursors[end-1], spec.filterDigest)
		if err != nil {
			return nil, nil, newRPCError(connect.CodeInternal, "failed to create page token", err)
		}
		response.NextPageToken = next
	}
	return items[start:end], response, nil
}
