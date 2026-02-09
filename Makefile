# ######################################################################################################################
# Dev Tools
# ######################################################################################################################
dev-setup:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/rakyll/gotest@latest
# ######################################################################################################################
# Tests
# ######################################################################################################################
test:
	CGO_ENABLED=0 gotest -v -race -count=1 ./...

test-integration:
	CGO_ENABLED=0 gotest -v -race -count=1 -tags=integration ./...

lint:
	CGO_ENABLED=0 go vet ./...
	staticcheck -checks=all ./...

vuln:
	govulncheck ./...

check: test lint vuln
# ######################################################################################################################
# Modules
# ######################################################################################################################
tidy:
	go mod tidy
	go mod vendor

deps-upgrade:
	# go get $(go list -f '{{if not (or .Main .Indirect)}}{{.Path}}{{end}}' -m all)
	go get -u -v ./...
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