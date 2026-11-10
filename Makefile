.PHONY: all build-ui build-backend build test clean run docker-build docker-up docker-down

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

docker-build:
	docker build -t swan-panel:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -f $(BINARY_NAME)
	rm -rf ui/dist
	rm -f *.db *.db-wal *.db-shm
