// Copyright 2018 The Prometheus Authors
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

// Package idemvfs implements a virtual file system that guarantees idempotency
// of modification time when a file content is identical to the stored state.
package idemvfs

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Identifier returns the identity of a file given its name. It should return
// false if the file is unknown.
type Identifier interface {
	Identify(name string) (Identity, bool)
}

// EmptyIdentifier implements Identifier, it always returns false.
type EmptyIdentifier struct{}

// Identify implements the Identifier interface.
func (n EmptyIdentifier) Identify(_ string) (Identity, bool) {
	return nil, false
}

// Identity defines the methods that represent the identity of a file.
type Identity interface {
	Checksum() []byte
	ModTime() time.Time
	Size() int64
}

// Equal returns true when the 2 files match.
func Equal(i1, i2 Identity) bool {
	return bytes.Compare(i1.Checksum(), i2.Checksum()) == 0 && i1.Size() == i2.Size()
}

// FileSystem implements the http.FileSystem interface. It returns idempotent
// modification times if the actual file's identity matches with the
// information returned by Identifier.
type FileSystem struct {
	fs http.FileSystem
	i  Identifier
}

// NewFileSystem creates a new FileSystem.
func NewFileSystem(fs http.FileSystem, i Identifier) *FileSystem {
	return &FileSystem{
		fs: fs,
		i:  i,
	}
}

// Open implements the http.FileSystem interface.
func (s *FileSystem) Open(name string) (http.File, error) {
	var err error
	f, err := s.fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	fstat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	var (
		sz1  int64
		chk1 []byte
	)
	if !fstat.IsDir() {
		// Compute the checksum and size of the actual file.
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil, fmt.Errorf("failed to compute checksum: %s", err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to rewind after checksum: %s", err)
		}

		chk1 = h.Sum(nil)
		sz1 = fstat.Size()
	}
	rf := &file{
		File:     f,
		FileInfo: fstat,
		ident: identity{
			fstat.ModTime(),
			sz1,
			chk1,
		},
	}

	ident, ok := s.i.Identify(name)
	if !ok {
		// The file isn't found, return the actual file.
		return rf, nil
	}

	if fstat.IsDir() || Equal(ident, rf) {
		rf.ident = identity{
			ident.ModTime(),
			ident.Size(),
			ident.Checksum(),
		}
	}

	return rf, nil
}

type identity struct {
	modTime  time.Time
	size     int64
	checksum []byte
}

type file struct {
	http.File
	os.FileInfo
	ident identity
}

// Stat implements the http.File interface.
func (f *file) Stat() (os.FileInfo, error) {
	return f, nil
}

// Stat implements the Identity and os.FileInfo interfaces.
func (f *file) ModTime() time.Time {
	return f.ident.modTime
}

// Checksum implements the Identity interface.
func (f *file) Checksum() []byte {
	return f.ident.checksum
}

// Size implements the Identity interface.
func (f *file) Size() int64 {
	return f.ident.size
}
