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

package store

import (
	"context"
	"errors"
	"sync"
	"time"
	//"os"
	//"log"
	//"fmt"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	//"encoding/json"
)

var (
	// ErrNotFound is returned if a Store cannot find the Alert.
	ErrNotFound = errors.New("alert not found")
)

// Alerts provides lock-coordinated to an in-memory map of alerts, keyed by
// their fingerprint. Resolved alerts are removed from the map based on
// gcInterval. An optional callback can be set which receives a slice of all
// resolved alerts that have been removed.
type Alerts struct {
	gcInterval time.Duration

	sync.Mutex
	c  map[model.Fingerprint]*types.Alert
	cb func([]*types.Alert)
}

// NewAlerts returns a new Alerts struct.
func NewAlerts(gcInterval time.Duration) *Alerts {
	if gcInterval == 0 {
		gcInterval = time.Minute
	}

	a := &Alerts{
		c:          make(map[model.Fingerprint]*types.Alert),
		cb:         func(_ []*types.Alert) {},
		gcInterval: gcInterval,
	}

	return a
}

// SetGCCallback sets a GC callback to be executed after each GC.
func (a *Alerts) SetGCCallback(cb func([]*types.Alert)) {
	a.Lock()
	defer a.Unlock()

	a.cb = cb
}

// Run starts the GC loop.
func (a *Alerts) Run(ctx context.Context) {
	go func(t *time.Ticker) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.gc()
			}
		}
	}(time.NewTicker(a.gcInterval))
}

func (a *Alerts) gc() {
	a.Lock()
	defer a.Unlock()

	resolved := []*types.Alert{}
	for fp, alert := range a.c {
		if alert.Resolved() {
			delete(a.c, fp)
			//delete(a.toggle,fp)
			resolved = append(resolved, alert)
		}
	}
	
	a.cb(resolved)
}

// Get returns the Alert with the matching fingerprint, or an error if it is
// not found.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.Lock()
	defer a.Unlock()

	alert, prs := a.c[fp]
	if !prs {
		return nil, ErrNotFound
	}
	return alert, nil
}
/* func (a *Alerts) GetToggle(fp model.Fingerprint) (int){
	a.Lock()
	defer a.Unlock()
	return a.toggle[fp]
	
}
func (a *Alerts) SetToggle(fp model.Fingerprint, value int) error {
	a.toggle[fp] = value
	return nil
} */
// Set unconditionally sets the alert in memory.
func (a *Alerts) Set(alert *types.Alert) error {
	a.Lock()
	defer a.Unlock()

	a.c[alert.Fingerprint()] = alert
	//a.toggle[alert.Fingerprint()] = 0
	return nil
}

// Delete removes the Alert with the matching fingerprint from the store.
func (a *Alerts) Delete(fp model.Fingerprint) error {
	a.Lock()
	defer a.Unlock()

	delete(a.c, fp)
	//delete(a.toggle, fp)
	return nil
}

// List returns a buffered channel of Alerts currently held in memory.
func (a *Alerts) List() <-chan *types.Alert {
	a.Lock()
	defer a.Unlock()

	c := make(chan *types.Alert, len(a.c))
	for _, alert := range a.c {
		c <- alert
	}
	close(c)

	return c
}

// Count returns the number of items within the store.
func (a *Alerts) Count() int {
	a.Lock()
	defer a.Unlock()

	return len(a.c)
}
/* type FileAlert struct {
	Alert *types.Alert 
	Status      string `json:"status"`
	Receivers   []string          `json:"receivers"`
	Fingerprint string            `json:"fingerprint"`
	TimeLog string `json:"timeLog"`
}
 */

/* func StoreAlert(alert FileAlert){

	fmt.Printf("Wrting alert")
	timestamp := int32(time.Now().Unix())
	times := fmt.Sprintf("%d", timestamp)
	date := time.Now().UTC().Format("01-02-2006")
	alert.TimeLog = times
	data, _ := json.MarshalIndent(alert, "", " ")
	var filename = "./Log_data/logAlert_" + date + ".json"
	_, err := os.Stat(filename)

	if err != nil {
		if os.IsNotExist(err){
			_, err := os.Create(filename)
			if err != nil {
				log.Fatal("Can't create log file", err)
			}
		}
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Can't open new file", err)
	}
	

	defer f.Close()

	/// if _, err = f.Write(times); err != nil {
	//	log.Fatal("Can't write timestamp to file", err)
	//} 
	if _, err = f.Write(data); err != nil {
		log.Fatal("Can't write to file", err)
	}
	fmt.Printf("Write data to file success!\n")
	

}
func DBAlert(alert FileAlert){
	
} */