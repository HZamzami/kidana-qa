# Kidana QA — task runner. Everything runs inside Docker; no Go/Node needed on the host.
# `just` with no arguments lists the recipes.

set dotenv-load := false

export HOST_UID := `id -u`
export HOST_GID := `id -g`

dc := "docker compose"
run := dc + " run --rm --no-deps app"

default:
    @just --list

# Build the dev image and start db + app (hot reload on http://localhost:8080)
up:
    {{dc}} up -d --build
    @echo "App: http://localhost:${APP_PORT:-8080}   (logs: just logs)"

# Stop the stack (data volumes are kept)
down:
    {{dc}} down

# Follow app logs
logs service="app":
    {{dc}} logs -f {{service}}

# Open a shell in the dev container
shell:
    {{dc}} run --rm app bash

# Regenerate templ components and sqlc queries
gen:
    {{run}} sh -c "templ generate && sqlc generate"

# Build the Tailwind stylesheet once
css:
    {{run}} tailwindcss -i web/styles/input.css -o web/static/css/app.css --minify

# Format Go and templ sources
fmt:
    {{run}} sh -c "templ fmt . && gofmt -w ./cmd ./internal"

# Run golangci-lint and check generated code is up to date
lint:
    {{run}} sh -c "templ generate && sqlc generate && golangci-lint run ./..."

# Run all tests (integration tests use the kidana_qa_test database)
test *args:
    {{dc}} run --rm app sh -c "templ generate && go test {{ if args == '' { './...' } else { args } }}"

# Apply migrations to the dev database (the app also does this on start)
migrate:
    {{dc}} run --rm app sh -c 'goose -dir internal/db/migrations postgres "$DATABASE_URL" up'

# Show migration status
migrate-status:
    {{dc}} run --rm app sh -c 'goose -dir internal/db/migrations postgres "$DATABASE_URL" status'

# Create a new SQL migration: just migration add_something
migration name:
    {{run}} goose -dir internal/db/migrations create {{name}} sql

# psql into the dev database
psql:
    {{dc}} exec db psql -U kidana -d kidana_qa

# Tidy Go modules
tidy:
    {{run}} go mod tidy

# Build the production image
image tag="kidana-qa:latest":
    docker build --target prod -t {{tag}} .

# Delete the stack AND its volumes (database, uploads, caches)
nuke:
    {{dc}} down -v

# Screenshot key screens (phone + desktop) via Playwright and report browser console errors.
# Needs the stack running (`just up`). Output: tmp/ui-check/
ui-check:
    mkdir -p tmp/ui-check && chmod 777 tmp/ui-check
    docker run --rm --network host --env-file .env -v "$PWD/scripts:/scripts:ro,z" -v "$PWD/tmp/ui-check:/out:z" -w /tmp \
        mcr.microsoft.com/playwright/python:v1.55.0-noble \
        sh -c 'pip install -q --break-system-packages playwright==1.55.0 openpyxl==3.1.5 2>/dev/null; python3 /scripts/ui_check.py /out'
