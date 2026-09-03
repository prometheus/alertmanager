SRC_FILES = $(shell find src test-utils -type f)
PROTO_FILES = $(shell find ../../proto/api -type f -name '*.proto')

node_modules: package.json package-lock.json
	@echo ">> installing Mantine UI dependencies"
	npm ci

.PHONY: build
build: ../app/dist/mantine/.build_stamp

# .build_stamp allows skipping the build inside `promu crossbuild`, since the
# artifact copies the work tree into the build container.
../app/dist/mantine/.build_stamp: $(SRC_FILES) $(PROTO_FILES) buf.gen.yaml index.html vite.config.mjs postcss.config.cjs tsconfig.json package-lock.json
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
