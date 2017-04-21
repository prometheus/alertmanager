ELM_FILES := $(shell find src -iname *.elm)
format: $(ELM_FILES)
	elm-format --yes $?

test:
	elm-test

dev-server:
	go run main.go --debug
