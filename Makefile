.PHONY: all build-ui build-backend build test clean run

BINARY_NAME=swan-panel

all: build

build-ui:
	cd ui && npm run build

build-backend:
	go build -o $(BINARY_NAME) ./cmd/server

build: build-ui build-backend

test:
	go test -v -race ./...

run: build
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	rm -rf ui/dist
	rm -f *.db *.db-wal *.db-shm
