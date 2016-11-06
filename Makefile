format: $(wildcard src/*.elm)
	elm-format --yes $?
