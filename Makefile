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
	CGO_ENABLED=1 go tool gotest -v -race -count=1 ./...

test-integration:
	CGO_ENABLED=0 go tool gotest -v -race -count=1 -tags=integration ./...

lint:
	CGO_ENABLED=0 go vet ./...
	go tool staticcheck -checks=all ./...

vuln:
	go tool govulncheck ./...

check: test lint vuln
# ######################################################################################################################
# Docs
# ######################################################################################################################
docs:
	@echo "Starting pkgsite at http://localhost:6060/github.com/adamwoolhether/httper/client"
	@open http://localhost:6060/github.com/adamwoolhether/httper/client &
	go tool pkgsite -http=localhost:6060
# ######################################################################################################################
# Modules
# ######################################################################################################################
tidy:
	go mod tidy
	go mod vendor

deps-upgrade:
	go get -u -v -tool ./...
	go mod tidy
	go mod vendor

deps-cleancache:
	go clean -modcache

deps-list:
	go list -m -u -mod=readonly all

deps-reset:
	git checkout -- go.mod
	go mod tidy
	go mod vendor
