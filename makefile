BINARY := pnp-powershell-mcp-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all clean tidy fmt vet lint check build run test install docker docker-run

all: build

clean:
	@rm -rf bin

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

check: fmt vet lint test

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

run: build
	@./bin/$(BINARY)

test:
	go test ./...

install:
	go install -ldflags "$(LDFLAGS)" .

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY) .

docker-run: docker
	docker run -i --rm $(BINARY)
