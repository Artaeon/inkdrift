.PHONY: build test clean install run docker

BINARY=inkdrift
VERSION=0.1.0

build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY) .

test:
	go test ./... -v

test-race:
	go test -race ./...

clean:
	rm -f $(BINARY)
	rm -f *.db

install: build
	sudo cp $(BINARY) /usr/local/bin/

run: build
	./$(BINARY) serve

docker:
	docker build -t inkdrift:$(VERSION) .

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

lint:
	go vet ./...
