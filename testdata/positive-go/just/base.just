init:
    go mod download

check:
    go test ./...

ci: check
