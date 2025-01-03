.PHONY: generate
generate:
	oapi-codegen -generate types,chi-server -package api api/openapi.yaml > internal/api/api.gen.go
	sqlc generate

.PHONY: build
build:
	go build -o bin/server cmd/server/main.go

.PHONY: run
run:
	go run cmd/server/main.go
