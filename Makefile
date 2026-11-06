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

# Match promu's version selection (VERSION file, else git describe) for the
# web UI tarball name and Makefile.common's DOCKER_MAJOR_VERSION_TAG.
ifneq ($(wildcard VERSION),)
  VERSION := $(strip $(file <VERSION))
  ifneq ($(wildcard .git),)
    $(warning Using version $(VERSION) from VERSION; remove the file to use git)
  endif
else ifneq ($(wildcard .git),)
  VERSION := $(patsubst v%,%,$(shell git describe --tags --always --dirty))
else
  # Source tarballs made by `git archive` have no git metadata, so they carry
  # the version in VERSION.in (export-subst). Write it to VERSION for promu.
  VERSION := $(patsubst v%,%,$(strip $(file <VERSION.in)))
VERSION: VERSION.in
	@case '$(VERSION)' in ''|'$$Format'*) echo "VERSION.in was not substituted by git archive; write the version to VERSION by hand." >&2; exit 1;; esac
	echo '$(VERSION)' > $@
common-build common-tarball assets-tarball: VERSION
endif
DOCKER_MAJOR_VERSION_TAG = $(firstword $(subst ., ,$(VERSION)))

FRONTEND_DIR             = $(BIN_DIR)/ui/app
MANTINE_UI_DIR            = $(BIN_DIR)/ui/mantine-ui
TEMPLATE_DIR             = $(BIN_DIR)/template
DOCKER_IMAGE_NAME       ?= alertmanager

STATICCHECK_IGNORE =

.PHONY: build-all
# Will build both the front-end as well as the back-end
build-all: assets apiv2
	$(MAKE) build

.PHONY: build
build: ui-elm ui-mantine common-build

.PHONY: test
test: ui-elm ui-mantine-test common-test

.PHONY: lint
lint: ui-elm ui-mantine-lint common-lint

.PHONY: assets
assets: $(FRONTEND_DIR)/src/Data ui-elm ui-mantine-assets template/email.tmpl

.PHONY: assets-tarball
assets-tarball: ui-elm ui-mantine-assets
	mkdir -p .tarballs
	tar czf ".tarballs/alertmanager-web-ui-$(VERSION).tar.gz" -C ui/app dist

.PHONY: ui-elm
ui-elm:
	cd $(FRONTEND_DIR) && $(MAKE) build

.PHONY: ui-mantine-assets
ui-mantine-assets:
	cd $(MANTINE_UI_DIR) && $(MAKE) assets

.PHONY: ui-mantine
ui-mantine:
	cd $(MANTINE_UI_DIR) && $(MAKE) build

.PHONY: ui-mantine-test
ui-mantine-test:
	cd $(MANTINE_UI_DIR) && $(MAKE) test

.PHONY: ui-mantine-lint
ui-mantine-lint:
	cd $(MANTINE_UI_DIR) && $(MAKE) lint

$(FRONTEND_DIR)/src/Data: api/v2/openapi.yaml
	cd $(FRONTEND_DIR) && $(MAKE) src/Data

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
	- @rm -rf template/email.tmpl \
                  api/v2/models api/v2/restapi api/v2/client
	- @cd $(FRONTEND_DIR) && $(MAKE) clean
	- @cd $(MANTINE_UI_DIR) && $(MAKE) clean
