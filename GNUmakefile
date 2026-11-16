BINARY = terraform-provider-reco

.PHONY: build install test cover testacc sweep generate lint

build:
	GOWORK=off go build -o $(BINARY) .

install:
	GOWORK=off go install .

test:
	GOWORK=off go test ./internal/... -count=1

cover:
	GOWORK=off ./scripts/check-coverage.sh

ACC_TIMEOUT ?= 120m

# Packages run one at a time so they never race each other on the shared tenant.
testacc:
	TF_ACC=1 GOWORK=off go test ./internal/... -v -p 1 -timeout $(ACC_TIMEOUT) -run "^TestAcc"

# Deletes tf-acc-* objects leaked by interrupted acceptance runs (needs RECO_API_KEY and RECO_BASE_URL).
sweep:
	GOWORK=off go test ./internal/resources -sweep=reco -timeout 30m

generate:
	GOWORK=off go generate ./...

lint:
	GOWORK=off golangci-lint run ./...
