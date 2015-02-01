# Copyright 2013 The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

include Makefile.INCLUDE

default: $(BINARY)

.deps/$(GOPKG):
	mkdir -p .deps
	curl -o .deps/$(GOPKG) -L $(GOURL)/$(GOPKG)

$(GOCC): .deps/$(GOPKG)
	tar -C .deps -xzf .deps/$(GOPKG)
	touch $@

$(SELFLINK):
	mkdir -p $(GOPATH)/src/github.com/prometheus
	ln -s $(CURDIR) $(SELFLINK)

dependencies: $(SELFLINK) web config
	$(GO) get -d

config:
	$(MAKE) -C config

web:
	$(MAKE) -C web

$(BINARY): $(GOCC) dependencies
	$(GO) build $(BUILDFLAGS) -o $@

$(ARCHIVE): $(BINARY)
	tar -czf $@ $<

release: REMOTE     ?= $(error "can't release, REMOTE not set")
release: REMOTE_DIR ?= $(error "can't release, REMOTE_DIR not set")
release: $(ARCHIVE)
	scp $< $(REMOTE):$(REMOTE_DIR)/$(ARCHIVE)

test: $(GOCC) dependencies
	$(GO) test ./...

clean:
	$(MAKE) -C web clean
	-rm -rf alertmanager .deps

.PHONY: clean config default dependencies release test web
