ELM_FILES := $(shell find src -iname *.elm)
format: $(ELM_FILES)
	elm-format --yes $?

test:
	elm-test

dev-server:
	elm-reactor

TEMPFILE := $(shell mktemp /tmp/elm-XXXXXXXXXX.js)
script.js: $(ELM_FILES)
	elm make src/Main.elm --yes --output $(TEMPFILE)
	uglifyjs $(TEMPFILE) --compress unused --mangle --output $(@)
	rm $(TEMPFILE)
