JSONNET_FMT := jsonnetfmt -n 2 --max-blank-lines 2 --string-style s --comment-style s
ALERTMANAGER_ALERTS := alertmanager_alerts.yaml

all: fmt $(ALERTMANAGER_ALERTS) lint

fmt:
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- $(JSONNET_FMT) -i

$(ALERTMANAGER_ALERTS): mixin.libsonnet alerts.libsonnet
	jsonnet -S alerts.jsonnet > $@

lint: $(ALERTMANAGER_ALERTS)
	find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		while read f; do \
			$(JSONNET_FMT) "$$f" | diff -u "$$f" -; \
		done

	promtool check rules $(ALERTMANAGER_ALERTS)

clean:
	rm -rf $(ALERTMANAGER_ALERTS)
