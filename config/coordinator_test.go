// Copyright 2019 Prometheus Team
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

package config

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
)

type fakeRegisterer struct {
	registeredCollectors []prometheus.Collector
}

func (r *fakeRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (r *fakeRegisterer) MustRegister(c ...prometheus.Collector) {
	r.registeredCollectors = append(r.registeredCollectors, c...)
}

func (r *fakeRegisterer) Unregister(prometheus.Collector) bool {
	return false
}

func TestCoordinatorRegistersMetrics(t *testing.T) {
	fr := fakeRegisterer{}
	NewCoordinator("testdata/conf.good.yml", &fr, promslog.NewNopLogger())

	if len(fr.registeredCollectors) == 0 {
		t.Error("expected NewCoordinator to register metrics on the given registerer")
	}
}

func TestCoordinatorNotifiesSubscribers(t *testing.T) {
	callBackCalled := false
	c := NewCoordinator("testdata/conf.good.yml", prometheus.NewRegistry(), promslog.NewNopLogger())
	c.Subscribe(func(*Config) error {
		callBackCalled = true
		return nil
	})

	err := c.Reload()
	if err != nil {
		t.Fatal(err)
	}

	if !callBackCalled {
		t.Fatal("expected coordinator.Reload() to call subscribers")
	}
}

func TestCoordinatorFailReloadWhenSubscriberFails(t *testing.T) {
	errMessage := "something happened"
	c := NewCoordinator("testdata/conf.good.yml", prometheus.NewRegistry(), promslog.NewNopLogger())

	c.Subscribe(func(*Config) error {
		return errors.New(errMessage)
	})

	err := c.Reload()
	if err == nil {
		t.Fatal("expected reload to throw an error")
	}

	if err.Error() != errMessage {
		t.Fatalf("expected error message %q but got %q", errMessage, err)
	}
}
