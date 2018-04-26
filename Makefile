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

ifdef DEBUG
	bindata_flags = -debug
endif

STATICCHECK_IGNORE = \
  github.com/prometheus/alertmanager/notify/notify.go:SA6002


.PHONY: build-all
# Will build both the front-end as well as the back-end
build-all: assets build

assets: go-bindata ui/bindata.go template/internal/deftmpl/bindata.go api/v2/models api/v2/restapi test/with_api_v2/api_v2_client/models test/with_api_v2/api_v2_client/client

go-bindata:
	-@$(GO) get -u github.com/jteeuwen/go-bindata/...

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
	rm template/internal/deftmpl/bindata.go
	rm ui/bindata.go
	cd $(FRONTEND_DIR) && $(MAKE) clean

.PHONY: test
test: common-test $(ERRCHECK_BINARY)
	@echo ">> running errcheck with exclude file scripts/errcheck_excludes.txt"
	$(ERRCHECK_BINARY) -verbose -exclude scripts/errcheck_excludes.txt -ignoretests ./...

$(ERRCHECK_BINARY):
	@go get github.com/kisielk/errcheck
