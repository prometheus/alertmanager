# Use `=` instead of `:=` expanding variable lazely, not at beginning. Needed as
# elm files change during execution.
ELM_FILES = $(shell find src -iname *.elm)
# If JUNIT_DIR is set, the tests are executed with the JUnit reporter and the result is stored in the given directory.
JUNIT_DIR           ?=
DOCKER              ?=
OPENAPI_GEN_IMAGE   ?= docker.io/openapitools/openapi-generator-cli:v3.3.4
UI_BUILD_IMAGE      ?= quay.io/prometheus/golang-builder:1.26-base #  Derived from ../../.promu.yml.
CA_BUNDLE           ?=

ifeq ($(DOCKER),)
  # Run locally.
  NPM = npm
  NPX = npx
  CONTAINER_RUNTIME = docker # Runtime is needed, since src/Data requires the OPENAPI_GEN_IMAGE.
  USER_ARGS = --user $(shell id -u):$(shell id -g)
else
  # Run npm and npx inside container runtime.
	# `USER_ARGS` prevents creation of root-owned files.
  ifeq ($(DOCKER),podman)
    USER_ARGS = --userns=keep-id --user $(shell id -u):$(shell id -g)
  else
    USER_ARGS = --user $(shell id -u):$(shell id -g)
  endif
  CA_BUNDLE_ARGS = $(if $(CA_BUNDLE),--volume $(CA_BUNDLE):/etc/ssl/certs/ca-certificates.crt:ro \
    --env NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt)
  DOCKER_RUN = $(DOCKER) run --rm -i \
    --entrypoint "" \
    -v "$(CURDIR):/app" \
    -w /app \
    $(USER_ARGS) \
    --env HOME=/tmp \
    $(CA_BUNDLE_ARGS) \
    $(UI_BUILD_IMAGE)
  NPM = $(DOCKER_RUN) npm
  NPX = $(DOCKER_RUN) npx
  CONTAINER_RUNTIME = $(DOCKER)
endif

.PHONY: all
all: src/Data build test

node_modules: package-lock.json
	@echo ">> installing dependencies"
	$(NPM) ci

.PHONY: format
format: node_modules $(ELM_FILES)
	@echo ">> format front-end code"
	@$(NPX) elm-format --yes $(ELM_FILES)

.PHONY: review
review: node_modules
	@$(NPX) elm-review --fix

.PHONY: test
test: node_modules
	rm -rf elm-stuff/generated-code
	@$(NPX) elm-format $(ELM_FILES) --validate
	@$(NPX) elm-review
ifneq ($(JUNIT_DIR),)
	mkdir -p $(JUNIT_DIR)
	@$(NPX) elm-test --report=junit | tee $(JUNIT_DIR)/junit.xml
else
	@$(NPX) elm-test
endif

.PHONY: dev-server
dev-server: node_modules
ifeq ($(DOCKER),)
	npx elm reactor
else
	$(DOCKER) run --rm -it \
	  --entrypoint "" \
	  -v "$(CURDIR):/app" \
	  -w /app \
	  -p 8000:8000 \
	  $(USER_ARGS) \
	  $(CA_BUNDLE_ARGS) \
	  $(UI_BUILD_IMAGE) \
	  npx elm reactor
endif

.PHONY: build
build: dist/.build_stamp

dist/.build_stamp: $(ELM_FILES) index.html vite.config.mjs package-lock.json
	@echo ">> building frontend"
	$(MAKE) node_modules
	$(NPM) run build
	@touch dist/.build_stamp

src/Data: ../../api/v2/openapi.yaml
	-rm -rf src/Data src/DateTime.elm
	$(CONTAINER_RUNTIME) run --rm \
	  $(USER_ARGS) \
	  --entrypoint sh \
	  -v ${PWD}/../..:/local \
	  -v ${PWD}/src:/output/src \
	  $(OPENAPI_GEN_IMAGE) \
	  -c 'java -jar /opt/openapi-generator/modules/openapi-generator-cli/target/openapi-generator-cli.jar generate -i /local/api/v2/openapi.yaml -g elm -o /tmp/openapi-gen && cp -r /tmp/openapi-gen/src/Data /output/src/Data && cp /tmp/openapi-gen/src/DateTime.elm /output/src/DateTime.elm'
	$(MAKE) format

.PHONY: clean
clean:
	- @rm -rf dist node_modules elm-stuff src/Data src/DateTime.elm openapi-* elm-*
