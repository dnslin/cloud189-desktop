SHELL := /bin/bash

GO_PACKAGES := ./...

.PHONY: test lint fmt vet check frontend-check

fmt:
	gofmt -w core cmd

vet:
	go vet $(GO_PACKAGES)

test:
	go test $(GO_PACKAGES) -v -race -coverprofile=coverage.out -covermode=atomic

lint:
	golangci-lint run ./core/...

check: fmt vet lint test frontend-check

frontend-check:
	./scripts/check_frontend.sh
