.PHONY: build test install

build:
	go build -o prui ./cmd/prui

test:
	go test ./...

install:
	go install ./cmd/prui
