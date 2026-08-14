.PHONY: test lint build coverage eval web-build web-test

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

build:
	go build ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

eval:
	go run ./cmd/eval/

web-build:
	cd web && npm run build

web-test:
	cd web && npm test
