.PHONY: fmt vet lint test build migrate server

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test ./...

build:
	go build -o go-uptime .

migrate:
	go run . db-goose-migrate

server:
	go run . server
