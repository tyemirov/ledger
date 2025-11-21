.PHONY: ci test backend-test frontend-test configure-google-client-id

ci: backend-test frontend-test

test: ci

backend-test:
	cd backend && go test ./...

frontend-test:
	cd ui && npm test

configure-google-client-id:
	@if [ -z "$(GOOGLE_CLIENT_ID)" ]; then \
		echo "GOOGLE_CLIENT_ID is required, e.g. make configure-google-client-id GOOGLE_CLIENT_ID=xxx.apps.googleusercontent.com"; \
		exit 1; \
	fi
	node scripts/set-google-client-id.js "$(GOOGLE_CLIENT_ID)"
