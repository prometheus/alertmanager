# Use `=` instead of `:=` expanding variable lazely, not at beginning. Needed as
# elm files change during execution.
ELM_FILES = $(shell find src -iname *.elm)
DOCKER_IMG := elm-env
DOCKER_RUN_CURRENT_USER := docker run --user=$(shell id -u $(USER)):$(shell id -g $(USER))
DOCKER_CMD := $(DOCKER_RUN_CURRENT_USER) --rm -t -v $(PWD):/app -w /app -e "ELM_HOME=/app/.elm" $(DOCKER_IMG)
# If JUNIT_DIR is set, the tests are executed with the JUnit reporter and the result is stored in the given directory.
JUNIT_DIR ?=

ifeq ($(NO_DOCKER), true)
  DOCKER_CMD=
endif

all: script.js test

elm-env:
	@(if [ "$(NO_DOCKER)" != "true" ] ; then \
		echo ">> building elm-env docker image"; \
		docker build --platform=linux/amd64 -t $(DOCKER_IMG) ../. > /dev/null; \
	fi; )

format: elm-env $(ELM_FILES)
	@echo ">> format front-end code"
	@$(DOCKER_CMD) elm-format --yes $(ELM_FILES)

review: src/Data elm-env
	@$(DOCKER_CMD) elm-review --fix

test: src/Data elm-env
	@$(DOCKER_CMD) rm -rf elm-stuff/generated-code
	@$(DOCKER_CMD) elm-format $(ELM_FILES) --validate
	@$(DOCKER_CMD) elm-review
ifneq ($(JUNIT_DIR),)
	mkdir -p $(JUNIT_DIR)
	@$(DOCKER_CMD) elm-test --report=junit | tee $(JUNIT_DIR)/junit.xml
else
	@$(DOCKER_CMD) elm-test
endif

dev-server:
	elm reactor

# macOS requires mktemp template to be at the end of the filename,
# however --output flag for elm make must end in .js or .html.
script.js: export TEMPFILE := "$(shell mktemp ./elm-XXXXXXXXXX)"
script.js: export TEMPFILE_JS := "$(TEMPFILE).js"
script.js: src/Data elm-env format $(ELM_FILES)
	@echo ">> building script.js"
	@$(DOCKER_CMD) rm -rf elm-stuff
	@$(DOCKER_CMD) elm make src/Main.elm --optimize --output $(TEMPFILE_JS)
	@$(DOCKER_CMD) uglifyjs $(TEMPFILE_JS) --compress 'pure_funcs="F2,F3,F4,F5,F6,F7,F8,F9,A2,A3,A4,A5,A6,A7,A8,A9",pure_getters,keep_fargs=false,unsafe_comps,unsafe' --mangle --output $(@)
	@rm -rf $(TEMPFILE_JS)
	@rm -rf $(TEMPFILE)

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


clean:
	- @rm -rf script.js elm-stuff src/Data src/DateTime.elm openapi-* elm-*
	- @if [ ! -z "$(docker images -q $(DOCKER_IMG))" ]; then \
             docker rmi $(DOCKER_IMG); fi
