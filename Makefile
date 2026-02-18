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

# Needs to be defined before including Makefile.common to auto-generate targets
DOCKER_ARCHS ?= amd64 armv7 arm64 ppc64le s390x

include Makefile.common

FRONTEND_DIR             = $(BIN_DIR)/ui/app
TEMPLATE_DIR             = $(BIN_DIR)/template
DOCKER_IMAGE_NAME       ?= alertmanager

STATICCHECK_IGNORE =

.PHONY: build-all
# Will build both the front-end as well as the back-end
build-all: assets apiv2 build

.PHONY: build
build: common-build

.PHONY: lint
lint: common-lint

.PHONY: assets
assets: asset/assets_vfsdata.go

asset/assets_vfsdata.go: ui/app/script.js ui/app/index.html ui/app/lib template/default.tmpl template/email.tmpl
	$(GO) generate $(GOOPTS) ./asset
	@$(GOFMT) -w ./asset

ui/app/script.js: $(shell find ui/app/src -iname *.elm) api/v2/openapi.yaml
	cd $(FRONTEND_DIR) && $(MAKE) script.js

template/email.tmpl: template/email.html
	cd $(TEMPLATE_DIR) && $(MAKE) email.tmpl

.PHONY: apiv2
apiv2: api/v2/models api/v2/restapi api/v2/client


api/v2/models api/v2/restapi api/v2/client:  api/v2/openapi.yaml
	scripts/swagger.sh

.PHONY: fuzz-config
fuzz-config:
	go test -fuzz=^Fuzz -fuzztime=5s ./config

.PHONY: clean
clean:
	- @rm -rf asset/assets_vfsdata.go \
                  template/email.tmpl \
                  api/v2/models api/v2/restapi api/v2/client
	- @cd $(FRONTEND_DIR) && $(MAKE) clean
