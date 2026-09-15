node_modules: package-lock.json
	npm ci

email.tmpl: email.html inline-css.js node_modules
	@echo ">> inline css for html email template"
	node ./inline-css
