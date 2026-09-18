SRC_FILES = $(shell find src test-utils -type f)
BUILD_DEPS = \
	Makefile \
	index.html \
	package.json \
	package-lock.json \
	postcss.config.cjs \
	tsconfig.json \
	vite.config.mjs

node_modules: package.json package-lock.json
	@echo ">> installing Mantine UI dependencies"
	npm ci

.PHONY: generate
generate: node_modules
	npm run generate

.PHONY: assets
assets: generate
	$(MAKE) build

.PHONY: build
build: ../app/dist/mantine/.build_stamp

# .build_stamp allows skipping the build inside `promu crossbuild`, since the
# artifact copies the work tree into the build container.
../app/dist/mantine/.build_stamp: $(SRC_FILES) $(BUILD_DEPS)
	@echo ">> building Mantine UI"
	$(MAKE) node_modules
	npm run build
	@touch ../app/dist/mantine/.build_stamp

.PHONY: test
test: node_modules
	npm run vitest

.PHONY: lint
lint: node_modules
	npm run check

.PHONY: clean
clean:
	- @rm -rf ../app/dist/mantine node_modules
