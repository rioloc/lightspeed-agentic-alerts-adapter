BINARY_NAME := lightspeed-agentic-alerts-adapter
IMAGE_NAME := quay.io/openshift-lightspeed/$(BINARY_NAME)
IMAGE_TAG ?= latest

.PHONY: build test vet lint image-build clean

build:
	go build -o bin/$(BINARY_NAME) ./cmd

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

image-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

clean:
	rm -rf bin/
