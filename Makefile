CONFIG ?= configs/config.yaml

.PHONY: run backtest test build lint tidy

run:
	go run ./cmd/bot/... -config $(CONFIG)

backtest:
	go run ./cmd/backtest/... -config $(CONFIG)

test:
	go test ./... -race -count=1

build:
	go build -o bin/bot ./cmd/bot/...
	go build -o bin/backtest ./cmd/backtest/...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
