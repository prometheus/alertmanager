// Copyright 2020 Prometheus Team
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
package cluster

import (
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"
)

type connectionPool struct {
	connections map[string]*tlsConn
	tlsConfig   *tls.Config
	lock        sync.Mutex
}

func newConnectionPool(tlsClientCfg *tls.Config) *connectionPool {
	return &connectionPool{
		connections: make(map[string]*tlsConn),
		tlsConfig:   tlsClientCfg,
	}
}

// borrowConnection returns a *tlsConn from the pool. The connection does not
// need to be returned to the pool because each connection has its own locking.
func (pool *connectionPool) borrowConnection(addr string, timeout time.Duration) (*tlsConn, error) {
	pool.lock.Lock()
	defer pool.lock.Unlock()
	if pool.connections == nil {
		return nil, errors.New("connection pool closed")
	}
	var err error
	key := fmt.Sprintf("%s/%d", addr, int64(timeout))
	conn, ok := pool.connections[key]
	if !ok || !conn.alive() {
		conn, err = dialTLSConn(addr, timeout, pool.tlsConfig)
		if err != nil {
			return nil, err
		}
		pool.connections[key] = conn
	}
	return conn, nil
}

func (pool *connectionPool) shutdown() {
	pool.lock.Lock()
	defer pool.lock.Unlock()
	for key, conn := range pool.connections {
		if conn != nil {
			_ = conn.Close()
		}
		delete(pool.connections, key)
	}
	pool.connections = nil
}
