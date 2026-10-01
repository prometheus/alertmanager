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

package dispatch

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

// keyHash computes the opaque key of a route or of an aggregation group from
// its readable path and its receiver. Route paths and group paths are
// canonical: matchers are sorted and quoted, and so are group labels, so
// equal keys mean equal path and receiver. Nothing else, such as timings,
// time intervals, continue, group_by or the position among siblings, is
// hashed, so the key is stable under unrelated edits and reorderings.
func keyHash(path, receiver string) string {
	h := sha256.New()
	writeHashField(h, path)
	writeHashField(h, receiver)
	return hex.EncodeToString(h.Sum(nil))
}

// writeHashField writes s to h prefixed with its length, so that the
// boundary between consecutive fields is unambiguous: without it, the pair
// ("ab", "c") would hash like ("a", "bc"). A separator byte would not do, as
// matcher values and label values may contain any byte.
func writeHashField(h hash.Hash, s string) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(len(s)))
	// hash.Hash.Write never returns an error.
	h.Write(buf[:n])
	h.Write([]byte(s))
}
