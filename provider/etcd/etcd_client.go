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

package etcd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"go.etcd.io/etcd/clientv3"
	"google.golang.org/grpc"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

var (
	ErrorEtcdNotInitialized     = errors.New("Etcd not initialized")
	ErrorEtcdGetNoResult        = errors.New("etcdGet did not receive a result for fingerprint")
	ErrorEtcdGetMultipleResults = errors.New("etcdGet received multiple results for fingerprint")
)

const EtcdTimeoutGet = 150 * time.Millisecond
const EtcdTimeoutPut = 250 * time.Millisecond
const EtcdDelayRunWatch = 10 * time.Second
const EtcdDelayRunLoad = 15 * time.Second
const EtcdRetryGetFailure = 5 * time.Second

type EtcdClient struct {
	alerts    *Alerts
	endpoints []string
	prefix    string
	logger    log.Logger
	client    *clientv3.Client
	mtx       sync.Mutex
}

func NewEtcdClient(ctx context.Context, a *Alerts, endpoints []string, prefix string) (*EtcdClient, error) {

	ec := &EtcdClient{
		alerts:    a,
		endpoints: endpoints,
		prefix:    prefix,
		logger:    log.With(a.logger, "component", "provider.etcd"),
	}

	// create the configuration
	etcdConfig := clientv3.Config{
		Endpoints:        endpoints,
		AutoSyncInterval: 60 * time.Second,
		DialTimeout:      10 * time.Second,
		DialOptions:      []grpc.DialOption{grpc.WithBlock()}, // block until connect
	}

	// create the client
	client, err := clientv3.New(etcdConfig)
	if err != nil {
		// On startup, if we cannot connect to the etcd cluster, then fail hard so that the
		// user may address a potential configuration issue.  Once the clientv3 connects
		// successfully, clientv3 will reconnect to the etcd cluster as it goes down or up,
		// or into or out of network connectivity.
		level.Error(ec.logger).Log("msg", "Etcd connection failed", "err", err)
		os.Exit(1)
	} else {
		level.Info(ec.logger).Log("msg", "Etcd connection successful")
	}
	ec.mtx.Lock()
	ec.client = client
	ec.mtx.Unlock()

	// start a goroutine to ensure the client will be cleaned up when the context is done
	go func() {
		defer func() {
			ec.mtx.Lock()
			if ec.client != nil {
				ec.client.Close()
				ec.client = nil
			}
			ec.mtx.Unlock()
			level.Info(ec.logger).Log("msg", "Etcd connection shut down")
		}()

		for range ctx.Done() {
		}
	}()
	return ec, nil
}

func (ec *EtcdClient) CheckAndPut(alert *types.Alert) error {
	// Reduce writes to Etcd.  Only put to Etcd if the current alert is different than the same
	// alert in etcd (excluding/ignoring the alert.UpdatedAt field).  To do this we:
	//   1) Fetch the alert with the same fingerprint from Etcd
	//   2) Compare our current alert with the alert from Etcd.
	//   3) If the alerts are different, then write it back.
	//
	// This mechanism is simple and convergent. It is more effiencient to get from Etcd, check,
	// then put into Etcd because unnecessarily putting any alert into etcd will result in the
	// put alert being sent to all the AMs which are watching etcd.

	etcdAlert, err := ec.Get(alert.Fingerprint())
	if err != nil && err != ErrorEtcdGetNoResult {
		// it's ok if the result doesn't exist; all other errors are fatal
		return err
	} else if AlertsEqualExceptForUpdatedAt(etcdAlert, alert) {
		return nil // skip write to etcd
	}

	if len(alert.Labels) == 0 {
		// TODO: Saw this case happen.  Unsure if it was due to someone curling against AM.
		//   For now, skip etcd write so it doesn't propogate to other AMs
		level.Warn(ec.logger).Log("msg", "Skipping write of alert with empty LabelSet")
		return nil // skip write to etcd
	}

	return ec.Put(alert)
}

func (ec *EtcdClient) Get(fp model.Fingerprint) (*types.Alert, error) {
	// We do a best effort.  If etcd is not initialized yet, then skip
	if ec.client == nil {
		level.Error(ec.logger).Log("msg", "Not getting alert from etcd, etcd not initialized yet")
		return nil, ErrorEtcdNotInitialized
	}

	// ensure the operation does not take too long
	ctx, cancel := context.WithTimeout(context.Background(), EtcdTimeoutGet)
	defer cancel()

	ec.mtx.Lock()
	resp, err := ec.client.Get(ctx, ec.prefix+fp.String())
	ec.mtx.Unlock()
	if err != nil {
		level.Error(ec.logger).Log("msg", "Error getting alert from etcd", "err", err)
		return nil, err
	}

	if len(resp.Kvs) == 0 {
		return nil, ErrorEtcdGetNoResult
	} else if len(resp.Kvs) != 1 {
		return nil, ErrorEtcdGetMultipleResults
	}

	alert, err := UnmarshalAlert(string(resp.Kvs[0].Value))
	if err != nil {
		level.Error(ec.logger).Log("msg", "Error unmarshaling JSON Alert", "err", err)
		return nil, err
	}

	return alert, nil
}

func (ec *EtcdClient) Put(alert *types.Alert) error {
	// We do a best effort.  If etcd is not initialized yet, then skip
	if ec.client == nil {
		level.Error(ec.logger).Log("msg", "Not putting alert to etcd, etcd not initialized yet")
		return ErrorEtcdNotInitialized
	}

	fp := alert.Fingerprint()
	alertStr, err := MarshalAlert(alert)
	if err != nil {
		level.Error(ec.logger).Log("msg", "Error marshaling JSON Alert", "err", err)
		return err
	}

	// ensure the operation does not take too long
	ctx, cancel := context.WithTimeout(context.Background(), EtcdTimeoutPut)
	defer cancel()

	ec.mtx.Lock()
	_, err = ec.client.Put(ctx, ec.prefix+fp.String(), alertStr)
	ec.mtx.Unlock()
	if err != nil {
		level.Error(ec.logger).Log("msg", "Error putting alert to etcd", "err", err)
		return err
	}
	return nil
}

func (ec *EtcdClient) Del(fp model.Fingerprint) error {
	// We do a best effort.  If etcd is not initialized yet, then skip
	if ec.client == nil {
		level.Error(ec.logger).Log("msg", "Not deleting alert from etcd, etcd not initialized yet")
		return ErrorEtcdNotInitialized
	}

	// ensure the operation does not take too long
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ec.mtx.Lock()
	_, err := ec.client.Delete(ctx, ec.prefix+fp.String())
	ec.mtx.Unlock()
	if err != nil {
		return err
	}
	return nil
}

func (ec *EtcdClient) RunWatch(ctx context.Context) {
	// watch for alert changes in etcd and writes them back to our
	// local alert state
	ctx = clientv3.WithRequireLeader(ctx)

	go func() {
		ec.mtx.Lock()
		rch := ec.client.Watch(ctx, ec.prefix, clientv3.WithPrefix())
		ec.mtx.Unlock()

		level.Info(ec.logger).Log("msg", "Etcd Watch Started")
		for wresp := range rch {
			for _, ev := range wresp.Events {
				level.Debug(ec.logger).Log("msg", "watch received",
					"type", ev.Type, "key", fmt.Sprintf("%q", ev.Kv.Key), "value", fmt.Sprintf("%q", ev.Kv.Value))
				if ev.Type.String() == "PUT" {
					alert, err := UnmarshalAlert(string(ev.Kv.Value))
					if err != nil {
						continue
					}
					if len(alert.Labels) == 0 {
						// TODO: Saw this case happen.  Unsure if it was due to someone curling against AM.
						//   For now, skip loading of this alert
						level.Warn(ec.logger).Log("msg", "Watch received Unmarshalled alert with empty LabelSet")
						continue
					}
					_ = ec.alerts.Put(alert) // best effort only
				} // else, ignore all other etcd operations, especially DELETE
			}
		}
	}()
}

func (ec *EtcdClient) RunLoadAllAlerts(ctx context.Context) {
	go func() {
		level.Info(ec.logger).Log("msg", "Etcd Load All Alerts Started")
		for {
			ec.mtx.Lock()
			resp, err := ec.client.Get(ctx, ec.prefix, clientv3.WithPrefix())
			ec.mtx.Unlock()
			if err != nil {
				level.Error(ec.logger).Log("msg", "Error fetching all alerts etcd", "err", err)
				time.Sleep(EtcdRetryGetFailure)
				continue // retry
			}

			for _, ev := range resp.Kvs {
				level.Debug(ec.logger).Log("msg", "get received",
					"key", fmt.Sprintf("%q", ev.Key), "value", fmt.Sprintf("%q", ev.Value))
				alert, err := UnmarshalAlert(string(ev.Value))
				if err != nil {
					continue // retry
				}
				_ = ec.alerts.Put(alert) // best effort only
			}
			level.Info(ec.logger).Log("msg", "Etcd Load All Alerts Finished")
			return // we only need to load all of the alerts once
		}
	}()
}

// Equals returns a true if the two alerts have the same values, false otherwise
func AlertsEqualExceptForUpdatedAt(a *types.Alert, o *types.Alert) bool {
	if a == nil || o == nil {
		return false
	}
	if !reflect.DeepEqual(a.Labels, o.Labels) {
		return false
	}
	if !reflect.DeepEqual(a.Annotations, o.Annotations) {
		return false
	}
	if a.GeneratorURL != o.GeneratorURL {
		return false
	}
	if !a.StartsAt.Equal(o.StartsAt) {
		return false
	}
	if !a.EndsAt.Equal(o.EndsAt) {
		return false
	}
	// we explicitly ignore UpdatedAt
	// if !a.UpdatedAt.Equal(o.UpdatedAt) {
	// 	return false
	// }
	return a.Timeout == o.Timeout
}

func MarshalAlert(alert *types.Alert) (string, error) {
	b, err := json.Marshal(alert)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func UnmarshalAlert(alertStr string) (*types.Alert, error) {
	var alert types.Alert
	err := json.Unmarshal([]byte(alertStr), &alert)
	if err != nil {
		return nil, err
	}
	return &alert, nil
}
