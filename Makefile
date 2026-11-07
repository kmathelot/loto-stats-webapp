BIN := bin/loto-stats
IMAGE ?= loto-stats:dev

.PHONY: run build test vet fmt docker

run: ## run locally, auth disabled, on :8080
	go run ./cmd/loto-stats

build:
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/loto-stats

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

docker:
	docker build -t $(IMAGE) .
