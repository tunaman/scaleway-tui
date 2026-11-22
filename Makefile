BINARY_NAME=scw-tui
# Keep in sync with .github/workflows/security.yml.
GOSEC_VERSION=v2.30.0
GOVULNCHECK_VERSION=v1.9.0

# gosec lives in GOPATH/bin after `make install-tools`; prefer a PATH copy.
GOSEC ?= $(shell command -v gosec 2>/dev/null || echo "$(shell go env GOPATH)/bin/gosec")

build:
	go mod tidy
	go build -o bin/$(BINARY_NAME) .

# Unit tests + vet (CI runs go vet in the lint job).
test:
	go vet ./...
	go test .

# Linters + security scans (matches CI: golangci-lint, gosec, govulncheck).
lint:
	golangci-lint run
	@test -x "$(GOSEC)" || { echo "gosec not found, run: make install-tools"; exit 1; }
	"$(GOSEC)" ./...
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# Install local tools needed by the targets above.
install-tools:
	go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

clean:
	rm -rf bin/
