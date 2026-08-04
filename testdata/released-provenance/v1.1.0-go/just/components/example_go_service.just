[private]
_example_go_service_init:
    cd . && go mod download

[private]
_example_go_service_format:
    cd . && gofmt -w . && go tool goimports -w .

[private]
_example_go_service_format_check:
    #!/usr/bin/env sh
    set -eu
    cd .
    test -z "$(gofmt -l .)"
    test -z "$(go tool goimports -l .)"

[private]
_example_go_service_lint:
    cd . && GOTOOLCHAIN=local go vet ./... && golangci-lint run

[private]
_example_go_service_typecheck:
    cd . && GOTOOLCHAIN=local go test -mod=readonly -run '^$' ./...

[private]
_example_go_service_test:
    cd . && GOTOOLCHAIN=local go test -mod=readonly ./...

[private]
_example_go_service_build:
    cd . && mkdir -p bin && GOTOOLCHAIN=local CGO_ENABLED=0 go build -mod=readonly -trimpath -o bin/example-go-service ./cmd/example-go-service


[private]
_example_go_service_check:
    cd . && go mod tidy -diff && go mod verify

[private]
_example_go_service_ci:
    cd . && GOTOOLCHAIN=local go test -mod=readonly -race ./...
    cd . && test -x bin/example-go-service && test "$(bin/example-go-service)" = 'Example Go Service'

