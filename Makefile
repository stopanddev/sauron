.PHONY: build templ test init-env

# Create .env from .env.example with a random hub token (skipped if .env exists).
init-env:
	bash scripts/init-env.sh

build: templ
	go build -o bin/sauron ./cmd/sauron

templ:
	go run github.com/a-h/templ/cmd/templ@latest generate

test: templ
	go test ./...
