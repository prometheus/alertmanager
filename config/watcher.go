// Copyright 2013 Prometheus Team
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
	"github.com/golang/glog"
	"github.com/howeyc/fsnotify"
)

type ReloadCallback func(*Config)

type Watcher interface {
	Watch(c ReloadCallback)
}

type fileWatcher struct {
	fileName string
}

func NewFileWatcher(fileName string) *fileWatcher {
	return &fileWatcher{
		fileName: fileName,
	}
}

func (w *fileWatcher) Watch(cb ReloadCallback) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		glog.Fatal(err)
	}

	err = watcher.WatchFlags(w.fileName, fsnotify.FSN_MODIFY)
	if err != nil {
		glog.Fatal(err)
	}

	for {
		select {
		case ev := <-watcher.Event:
			glog.Infof("Config file changed (%s), attempting reload", ev)
			conf, err := LoadFromFile(w.fileName)
			if err != nil {
				glog.Error("Error loading new config: ", err)
				failedConfigReloads.Inc()
			} else {
				cb(&conf)
				glog.Info("Config reloaded successfully")
				configReloads.Inc()
			}
			// Re-add the file watcher since it can get lost on some changes. E.g.
			// saving a file with vim results in a RENAME-MODIFY-DELETE event
			// sequence, after which the newly written file is no longer watched.
			err = watcher.WatchFlags(w.fileName, fsnotify.FSN_MODIFY)
		case err := <-watcher.Error:
			glog.Error("Error watching config: ", err)
		}
	}
}
