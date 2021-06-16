.PHONY: build clean lint fmt test vet

GO_FILES                        = $(shell find . -name "*.go" | uniq)
GO_PACKAGES                     = $(shell go list ./... | grep -v "mock")

build:
	@mkdir -p build
	go mod tidy
	go mod download
	go build -o build/nextdhcp

clean:
	rm -r protobuf/v1/*
	rm -rf build

lint:
	golint -set_exit_status ./...

fmt:
	@echo "Go import/fmt"
	@goimports -w $(GO_FILES)

test:
	go test -cover ./...

vet:
	@echo "Go vet"
	go vet $(GO_PACKAGES)