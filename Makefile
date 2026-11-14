.PHONY: all build-ui build-backend build test lint check-release clean run docker-build docker-up docker-down release-static dev-check dev-up dev-down dev-logs

BINARY_NAME=swan-panel

all: build

build-ui:
	cd ui && npm run build

build-backend:
	CGO_ENABLED=1 go build -o $(BINARY_NAME) ./cmd/server

build: build-ui build-backend

test:
	CGO_ENABLED=1 go test -count=1 -race ./...

check-release:
	sh scripts/check-dependencies.sh
	sh scripts/check-descriptive-test-names.sh
	sh scripts/test-release-config.sh

lint: check-release
	CGO_ENABLED=1 golangci-lint run ./...

run: build
	./$(BINARY_NAME)

release-static: build-ui
	CGO_ENABLED=1 sh scripts/build-linux-static.sh

dev-check:
	node scripts/test-devcontainer-config.mjs
	docker compose -f .devcontainer/compose.yml config --quiet

dev-up: dev-check
	docker compose -f .devcontainer/compose.yml up --build -d

dev-down:
	docker compose -f .devcontainer/compose.yml down

dev-logs:
	docker compose -f .devcontainer/compose.yml logs -f --tail=100 workspace

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
