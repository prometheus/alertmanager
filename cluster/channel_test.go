// Copyright 2018 Prometheus Team
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
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/hashicorp/memberlist"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
)

func TestNormalMessagesGossiped(t *testing.T) {
	var sent bool
	c := newChannel(
		func(_ []byte) { sent = true },
		func() []*memberlist.Node { return nil },
		func(_ *memberlist.Node, _ []byte) error { return nil },
	)

	c.Broadcast([]byte{})

	if sent != true {
		t.Fatalf("small message not sent")
	}
}

func TestOversizedMessagesGossiped(t *testing.T) {
	var sent bool
	ctx, cancel := context.WithCancel(context.Background())
	c := newChannel(
		func(_ []byte) {},
		func() []*memberlist.Node { return []*memberlist.Node{{}} },
		func(_ *memberlist.Node, _ []byte) error { sent = true; cancel(); return nil },
	)

	f, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatalf("failed to open /dev/zero: %v", err)
	}
	defer f.Close()

	buf := new(bytes.Buffer)
	toCopy := int64(800)
	if n, err := io.CopyN(buf, f, toCopy); err != nil {
		t.Fatalf("failed to copy bytes: %v", err)
	} else if n != toCopy {
		t.Fatalf("wanted to copy %d bytes, only copied %d", toCopy, n)
	}

	c.Broadcast(buf.Bytes())

	<-ctx.Done()

	if sent != true {
		t.Fatalf("oversized message not sent")
	}
}

func newChannel(
	send func([]byte),
	peers func() []*memberlist.Node,
	sendOversize func(*memberlist.Node, []byte) error,
) *Channel {
	return NewChannel(
		"test",
		send,
		peers,
		sendOversize,
		promslog.NewNopLogger(),
		make(chan struct{}),
		prometheus.NewRegistry(),
	)
}
