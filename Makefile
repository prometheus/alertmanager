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

VERSION  := 0.0.4
TARGET   := alertmanager

include Makefile.COMMON

REV        := $(shell git rev-parse --short HEAD 2> /dev/null  || echo 'unknown')
BRANCH     := $(shell git rev-parse --abbrev-ref HEAD 2> /dev/null  || echo 'unknown')
HOSTNAME   := $(shell hostname -f)
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)
GOFLAGS    := -ldflags \
	"-X=main.buildVersion $(VERSION)\
	-X=main.buildRevision $(REV)\
	-X=main.buildBranch $(BRANCH)\
	-X=main.buildUser $(USER)@$(HOSTNAME)\
	-X=main.buildDate $(BUILD_DATE)\
	-X=main.goVersion $(GO_VERSION)"

web: web/blob/files.go

web/blob/files.go: $(shell find web/templates/ web/static/ -type f)
	./web/blob/embed-static.sh web/static web/templates | $(GOFMT) > $@

.PHONY: config
config:
	$(MAKE) -C config
