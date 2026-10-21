BINARY := pnp-powershell-mcp-server

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
	go build -o bin/$(BINARY) .

run: build
	@./bin/$(BINARY)

test:
	go test ./...

install:
	go install .

docker:
	docker build -t $(BINARY) .

docker-run: docker
	docker run -i --rm $(BINARY)
