GO_SOURCES := $(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./.git/*" -not -path "*/.git/*")
STATICCHECK_PACKAGES := $(shell go list ./... | grep -v github.com/MarkoPoloResearchLab/ledger/api/credit/v1)
UNIT_TEST_PACKAGES := $(shell go list ./... | grep -v github.com/MarkoPoloResearchLab/ledger/api/credit/v1)
INTEGRATION_TEST_PACKAGES :=
DEADCODE_ENTRYPOINT_PACKAGES := ./cmd/credit

.PHONY: fmt format check-format lint test test-unit test-integration ci tools check-unused-packages build-cgo-off

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
	staticcheck -tests=false $(STATICCHECK_PACKAGES)
	ineffassign $(UNIT_TEST_PACKAGES)
	errcheck -ignoretests $(UNIT_TEST_PACKAGES)
	deadcode $(DEADCODE_ENTRYPOINT_PACKAGES)
	$(MAKE) check-unused-packages
	$(MAKE) build-cgo-off

check-unused-packages:
	@set -eu; \
	module_path="$$(go list -m -f '{{.Path}}')"; \
	deps_file="$$(mktemp)"; \
	unused_file="$$(mktemp)"; \
	trap 'rm -f "$$deps_file" "$$unused_file"' EXIT; \
	go list -deps $(DEADCODE_ENTRYPOINT_PACKAGES) | grep "^$$module_path" | sort -u > "$$deps_file"; \
	for pkg in $$(go list ./...); do \
		if ! grep -Fxq "$$pkg" "$$deps_file"; then \
			echo "$$pkg" >> "$$unused_file"; \
		fi; \
	done; \
	if [ -s "$$unused_file" ]; then \
		echo "Packages not reachable from entrypoints ($(DEADCODE_ENTRYPOINT_PACKAGES)):"; \
		cat "$$unused_file"; \
		exit 1; \
	fi

build-cgo-off:
	@set -eu; \
	out_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$out_dir"' EXIT; \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$$out_dir/ledgerd" ./cmd/credit

test: test-unit

test-unit:
	go test $(UNIT_TEST_PACKAGES) -coverprofile=coverage.out -covermode=count
	go tool cover -func=coverage.out | awk 'END { if ($$3+0 < 100.0) { print "coverage below 100%"; exit 1 } }'

ci: check-format lint test-unit

tools:
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	@command -v ineffassign >/dev/null 2>&1 || go install github.com/gordonklaus/ineffassign@latest
	@command -v errcheck >/dev/null 2>&1 || go install github.com/kisielk/errcheck@latest
	@command -v deadcode >/dev/null 2>&1 || go install golang.org/x/tools/cmd/deadcode@latest
