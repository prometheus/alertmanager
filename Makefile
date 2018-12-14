# Copyright 2015 The Prometheus Authors
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

include Makefile.common

FRONTEND_DIR            = $(BIN_DIR)/ui/app
DOCKER_IMAGE_NAME       ?= alertmanager

ifdef DEBUG
	bindata_flags = -debug
endif

STATICCHECK_IGNORE = \
  github.com/prometheus/alertmanager/notify/notify.go:SA6002


.PHONY: build-all
# Will build both the front-end as well as the back-end
build-all: assets build

assets: go-bindata ui/bindata.go template/internal/deftmpl/bindata.go

go-bindata:
	-@$(GO) get -u github.com/jteeuwen/go-bindata/go-bindata

template/internal/deftmpl/bindata.go: template/default.tmpl
	@go-bindata $(bindata_flags) -mode 420 -modtime 1 -pkg deftmpl -o template/internal/deftmpl/bindata.go template/default.tmpl
	@$(GO) fmt ./template/internal/deftmpl

ui/bindata.go: ui/app/script.js ui/app/index.html ui/app/lib
# Using "-mode 420" and "-modtime 1" to make assets make target deterministic.
# It sets all file permissions and time stamps to 420 and 1
	@go-bindata $(bindata_flags) -mode 420 -modtime 1 -pkg ui -o \
		ui/bindata.go ui/app/script.js \
		ui/app/index.html \
		ui/app/favicon.ico \
		ui/app/lib/...
	@$(GO) fmt ./ui

ui/app/script.js: $(shell find ui/app/src -iname *.elm)
	cd $(FRONTEND_DIR) && $(MAKE) script.js

.PHONY: proto
proto:
	scripts/genproto.sh

.PHONY: clean
clean:
	rm template/internal/deftmpl/bindata.go
	rm ui/bindata.go
	cd $(FRONTEND_DIR) && $(MAKE) clean
