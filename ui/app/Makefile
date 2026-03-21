# Use `=` instead of `:=` expanding variable lazely, not at beginning. Needed as
# elm files change during execution.
ELM_FILES = $(shell find src -iname *.elm)
DOCKER_RUN_CURRENT_USER := docker run --user=$(shell id -u $(USER)):$(shell id -g $(USER))
# If JUNIT_DIR is set, the tests are executed with the JUnit reporter and the result is stored in the given directory.
JUNIT_DIR ?=

ifeq ($(NO_DOCKER), true)
  DOCKER_CMD=
endif

all: src/Data build test

node_modules: package-lock.json
	npm ci

format: node_modules $(ELM_FILES)
	@echo ">> format front-end code"
	@npx elm-format --yes $(ELM_FILES)

review: node_modules
	@npx elm-review --fix

test: node_modules
	rm -rf elm-stuff/generated-code
	@npx elm-format $(ELM_FILES) --validate
	@npx elm-review
ifneq ($(JUNIT_DIR),)
	mkdir -p $(JUNIT_DIR)
	@npx elm-test --report=junit | tee $(JUNIT_DIR)/junit.xml
else
	@npx elm-test
endif

dev-server:
	npx elm reactor

.PHONY: build
build: .build_stamp

.build_stamp: $(ELM_FILES) index.html vite.config.mjs node_modules
	@echo ">> building frontend"
	npm run build
	@touch .build_stamp

src/Data: export TEMPOPENAPI := $(shell mktemp -d ./openapi-XXXXXXXXXX)
src/Data: ../../api/v2/openapi.yaml
	-rm -r src/Data
	$(DOCKER_RUN_CURRENT_USER) --rm -v ${PWD}/../..:/local openapitools/openapi-generator-cli:v3.3.4 generate \
		-i /local/api/v2/openapi.yaml \
	  -g elm \
		-o /local/ui/app/$(TEMPOPENAPI)
	# We only want data directory & DateTime package.
	cp -r $(TEMPOPENAPI)/src/Data src/Data
	cp -r $(TEMPOPENAPI)/src/DateTime.elm src/DateTime.elm
	rm -rf $(TEMPOPENAPI)
	@npx elm-format --yes src/Data src/DateTime.elm


.PHONY: clean
clean:
	- @rm -rf .build_stamp dist elm-stuff src/Data src/DateTime.elm openapi-* elm-*
