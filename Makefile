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

FRONTEND_DIR             = $(BIN_DIR)/ui/app
DOCKER_IMAGE_NAME       ?= alertmanager
ERRCHECK_BINARY         := $(FIRST_GOPATH)/bin/errcheck

STATICCHECK_IGNORE =

# Go modules needs the bzr binary because of the dependency on launchpad.net/gocheck.
$(eval $(call PRECHECK_COMMAND_template,bzr))
PRECHECK_OPTIONS_bzr = version

.PHONY: build-all
# Will build both the front-end as well as the back-end
build-all: assets apiv2 build

assets: ui/app/script.js ui/app/index.html ui/app/lib template/default.tmpl
	cd $(PREFIX)/asset && $(GO) generate
	@$(GOFMT) -w ./asset

ui/app/script.js: $(shell find ui/app/src -iname *.elm) api/v2/openapi.yaml
	cd $(FRONTEND_DIR) && $(MAKE) script.js

.PHONY: apiv2
apiv2: api/v2/models api/v2/restapi test/with_api_v2/api_v2_client/models test/with_api_v2/api_v2_client/client

SWAGGER = docker run \
	--user=$(shell id -u $(USER)):$(shell id -g $(USER)) \
	--rm \
	-v $(shell pwd):/go/src/github.com/prometheus/alertmanager \
	-w /go/src/github.com/prometheus/alertmanager quay.io/goswagger/swagger:0.16.0

api/v2/models api/v2/restapi: api/v2/openapi.yaml
	-rm -r api/v2/{models,restapi}
	$(SWAGGER) generate server -f api/v2/openapi.yaml --exclude-main -A alertmanager --target api/v2/

test/with_api_v2/api_v2_client/models test/with_api_v2/api_v2_client/client: api/v2/openapi.yaml
	-rm -r test/with_api_v1/api_v2_client; mkdir -p test/with_api_v2/api_v2_client
	$(SWAGGER) generate client -f api/v2/openapi.yaml --target test/with_api_v2/api_v2_client

.PHONY: clean
clean:
	- rm -f asset/assets_vfsdata.go
	- rm -r api/v2/models api/v2/restapi test/with_api_v2/api_v2_client/models test/with_api_v2/api_v2_client/client
	- cd $(FRONTEND_DIR) && $(MAKE) clean

.PHONY: test
test: common-test $(ERRCHECK_BINARY)
	@echo ">> running errcheck with exclude file scripts/errcheck_excludes.txt"
	$(ERRCHECK_BINARY) -verbose -exclude scripts/errcheck_excludes.txt -ignoretests ./...

$(ERRCHECK_BINARY):
# Get errcheck from a temporary directory to avoid modifying the local go.{mod,sum}.
# See https://github.com/golang/go/issues/27643.
	tmpModule=$$(mktemp -d 2>&1) && \
	mkdir -p $${tmpModule}/staticcheck && \
	cd "$${tmpModule}"/staticcheck && \
	GO111MODULE=on $(GO) mod init example.com/staticcheck && \
	GO111MODULE=on GOOS= GOARCH= $(GO) get -u github.com/kisielk/errcheck && \
	rm -rf $${tmpModule};
