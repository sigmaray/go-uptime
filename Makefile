.PHONY: fmt vet lint lint-build test build migrate server

CUSTOM_GCL := ./custom-gcl

fmt:
	gofmt -w .

vet:
	go vet ./...

# Build a local golangci-lint binary that includes the NilAway module plugin.
lint-build:
	golangci-lint custom

lint: $(CUSTOM_GCL)
	$(CUSTOM_GCL) run ./...

$(CUSTOM_GCL): .custom-gcl.yml
	golangci-lint custom

test:
	go test ./...

build:
	go build -o go-uptime .

migrate:
	go run . db-goose-migrate

server:
	go run . server
