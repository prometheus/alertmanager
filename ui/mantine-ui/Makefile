node_modules: package.json package-lock.json
	@echo ">> installing Mantine UI dependencies"
	npm ci

.PHONY: build
build: node_modules
	@echo ">> building Mantine UI"
	npm run build

.PHONY: test
test: node_modules
	npm run vitest

.PHONY: lint
lint: node_modules
	npm run check

.PHONY: clean
clean:
	- @rm -rf ../app/dist/mantine node_modules
