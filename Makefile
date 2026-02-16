# ######################################################################################################################
# Dev Tools
# ######################################################################################################################
dev-setup:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/rakyll/gotest@latest
	go install golang.org/x/pkgsite/cmd/pkgsite@latest

# ######################################################################################################################
# Tests
# ######################################################################################################################
test:
	CGO_ENABLED=1 go -C client tool gotest -v -race -count=1 ./...
	CGO_ENABLED=1 go -C web tool gotest -v -race -count=1 ./...

test-integration:
	CGO_ENABLED=0 go -C client tool gotest -v -race -count=1 -tags=integration ./...
	CGO_ENABLED=0 go -C web tool gotest -v -race -count=1 -tags=integration ./...

test-verbose:
	CGO_ENABLED=1 VERBOSE=1 go -C client tool gotest -v -race -count=1 ./...
	CGO_ENABLED=1 VERBOSE=1 go -C web tool gotest -v -race -count=1 ./...

lint:
	CGO_ENABLED=0 go -C client vet ./...
	go -C client tool staticcheck -checks=all ./...
	CGO_ENABLED=0 go -C web vet ./...
	go -C web tool staticcheck -checks=all ./...

vuln:
	go -C client tool govulncheck ./...
	go -C web tool govulncheck ./...

check: test lint vuln
# ######################################################################################################################
# Docs
# ######################################################################################################################
docs-client:
	@echo "Starting pkgsite at http://localhost:6060/github.com/adamwoolhether/httper/client"
	@open http://localhost:6060/github.com/adamwoolhether/httper/client &
	go -C client tool pkgsite -http=localhost:6060
# ######################################################################################################################
# Modules
# ######################################################################################################################
tidy:
	go -C client mod tidy
	go -C web mod tidy

deps-upgrade:
	go -C client get -u -v -tool ./...
	go -C client mod tidy
	go -C web get -u -v -tool ./...
	go -C web mod tidy

deps-cleancache:
	go clean -modcache

deps-list:
	go -C client list -m -u -mod=readonly all
	go -C web list -m -u -mod=readonly all

deps-reset:
	git checkout -- client/go.mod client/go.sum
	go -C client mod tidy
	git checkout -- web/go.mod web/go.sum
	go -C web mod tidy
