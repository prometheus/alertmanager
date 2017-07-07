ELM_FILES := $(shell find src -iname *.elm)
DOCKER_IMG :=elm-env
DOCKER_CMD := docker run --rm -t -v $(PWD):/app -w /app $(DOCKER_IMG)
TEMPFILE := $(shell mktemp ./elm-XXXXXXXXXX.js)

ifeq ($(NO_DOCKER), true)
  DOCKER_CMD=
endif

all: test script.js

elm-env:
	@(if [ "$(NO_DOCKER)" != "true" ] ; then \
		echo ">> building elm-env docker image"; \
		docker build -t $(DOCKER_IMG) ../. > /dev/null; \
	fi; )

format: elm-env $(ELM_FILES)
	@echo ">> format front-end code"
	@$(DOCKER_CMD) elm-format --yes $(ELM_FILES)

test: elm-env
	@$(DOCKER_CMD) elm-format $(ELM_FILES) --validate
	@$(DOCKER_CMD) elm-test

dev-server:
	elm-reactor

script.js: elm-env format $(ELM_FILES)
	@echo ">> building script.js"
	@$(DOCKER_CMD) elm make src/Main.elm --yes --output $(TEMPFILE)
	@$(DOCKER_CMD)	uglifyjs $(TEMPFILE) --compress unused --mangle --output $(@)
	@rm $(TEMPFILE)

clean:
	- @rm script.js
	- @docker rmi $(DOCKER_IMG)
