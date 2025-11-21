.PHONY: ci test backend-test frontend-test

ci: backend-test frontend-test

test: ci

backend-test:
	cd backend && go test ./...

frontend-test:
	cd ui && npm test
