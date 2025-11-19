GO_SOURCES := $(shell find . -name '*.go' -not -path "./vendor/*")
GO_TEST_PATHS := ./cmd/... ./internal/... ./demo/...

ifneq ("$(wildcard tools/TAuth/pkg/sessionvalidator)","")
GO_TEST_PATHS += ./tools/TAuth/pkg/sessionvalidator
endif

ifneq ("$(wildcard tools/TAuth/internal/web)","")
GO_TEST_PATHS += ./tools/TAuth/internal/web
endif

.PHONY: fmt lint test ci tools

fmt:
	@if [ -z "$(GO_SOURCES)" ]; then exit 0; fi; \
	fmt_out="$$(gofmt -l $(GO_SOURCES))"; \
	if [ -n "$$fmt_out" ]; then \
		echo "Go files need formatting:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi

lint: tools
	go vet ./...
	staticcheck ./...
	ineffassign ./...

test:
	go test $(GO_TEST_PATHS)
	go test ./internal/credit -coverprofile=coverage.out -covermode=count
	go tool cover -func=coverage.out | awk 'END { if ($$3+0 < 80.0) { print "coverage below 80%"; exit 1 } }'
	npm ci
	npm run test:ui

ci: fmt lint test

tools:
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	@command -v ineffassign >/dev/null 2>&1 || go install github.com/gordonklaus/ineffassign@latest
