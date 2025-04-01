.PHONY: test
test:
	go test $(GOTEST_FLAGS) -race ./...

.PHONY: test-integration
test-integration:
	# run required docker containers, execute integration tests, stop containers after tests
	docker compose -f test/docker-compose-postgres.yml up --quiet-pull -d --wait
	go test $(GOTEST_FLAGS) -race --tags=integration ./...; ret=$$?; \
		docker compose -f test/docker-compose-postgres.yml down; \
		exit $$ret

.PHONY: lint
lint:
	golangci-lint run

.PHONY: fmt
fmt:
	gofumpt -l -w .

.PHONY: install-tools
install-tools:
	@echo Installing tools from tools/go.mod
	@go list -modfile=tools/go.mod tool | xargs -I % go list -modfile=tools/go.mod -f "%@{{.Module.Version}}" % | xargs -tI % go install %
	@go mod tidy

.PHONY: generate
generate:
	go generate ./...

.PHONY: proto-generate
proto-generate:
	cd proto && buf generate

.PHONY: proto-update
proto-update:
	cd proto && buf dep update

.PHONY: proto-lint
proto-lint:
	cd proto && buf lint
