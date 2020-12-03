JSONNET_FMT := jsonnetfmt -n 2 --max-blank-lines 2 --string-style s --comment-style s
ALERTMANAGER_ALERTS := alertmanager_alerts.yaml

default: build

all: fmt build

fmt:
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- $(JSONNET_FMT) -i

lint: build
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		while read f; do \
			$(JSONNET_FMT) "$$f" | diff -u "$$f" -; \
		done

	mixtool lint mixin.libsonnet

build:
	mixtool generate alerts mixin.libsonnet > $(ALERTMANAGER_ALERTS)

clean:
	rm -rf $(ALERTMANAGER_ALERTS)
