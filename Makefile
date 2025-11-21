GO_SOURCES := $(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./.git/*" -not -path "*/.git/*")
STATICCHECK_PACKAGES := $(shell go list ./... | grep -v github.com/MarkoPoloResearchLab/ledger/api/credit/v1)
UNIT_TEST_PACKAGES := $(shell go list ./... | grep -v github.com/MarkoPoloResearchLab/ledger/api/credit/v1)
INTEGRATION_TEST_PACKAGES :=

.PHONY: fmt format check-format lint test test-unit test-integration ci tools

fmt: check-format

format:
	@if [ -z "$(GO_SOURCES)" ]; then exit 0; fi; \
	gofmt -w $(GO_SOURCES)

check-format:
	@if [ -z "$(GO_SOURCES)" ]; then exit 0; fi; \
	fmt_out="$$(gofmt -l $(GO_SOURCES))"; \
	if [ -n "$$fmt_out" ]; then \
		echo "Go files need formatting:"; \
		echo "$$fmt_out"; \
		exit 1; \
	fi

lint: tools
	go vet $(UNIT_TEST_PACKAGES)
	staticcheck $(STATICCHECK_PACKAGES)
	ineffassign $(UNIT_TEST_PACKAGES)

test: test-unit

test-unit:
	go test $(UNIT_TEST_PACKAGES)
	go test ./pkg/ledger -coverprofile=coverage.out -covermode=count
	go tool cover -func=coverage.out | awk 'END { if ($$3+0 < 80.0) { print "coverage below 80%"; exit 1 } }'

ci: check-format lint test-unit

tools:
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	@command -v ineffassign >/dev/null 2>&1 || go install github.com/gordonklaus/ineffassign@latest
