BINARY = terraform-provider-reco

.PHONY: build install test testacc generate lint

build:
	GOWORK=off go build -o $(BINARY) .

install:
	GOWORK=off go install .

test:
	GOWORK=off go test ./internal/... -count=1

testacc:
	TF_ACC=1 GOWORK=off go test ./internal/... -v -timeout 120m -run "^TestAcc"

generate:
	GOWORK=off go generate ./...

lint:
	GOWORK=off golangci-lint run ./...
