##
# Greenlight
#
# @file
# @version 0.1

include .envrc
.DEFAULT_GOAL := build

.PHONY:vet build run help confirm clean db/psql db/migrations/new db/migrations/up audit

# ==================================================================================== #
# HELPERS
# ==================================================================================== #
## help: print this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #
vet:
	go vet ./...

build: vet
	go build ./cmd/api/

## run: run the cmd/api application
run: vet
	@go build ./cmd/api/ && ./api -db-dsn=${GREENLIGHT_DB_DSN}

clean:
	go clean -x

## db/psql: connect to the database using psql
db/psql:
	psql ${GREENLIGHT_DB_DSN}

## db/migrations/new name=$1: create a new database migration
db/migrations/new:
	@echo 'Creating migrationfiles for ${name}'
	migrate create -seq -ext .sql -dir=./migrations ${name}

## db/migrations/up: apply all up database migrations
db/migrations/up: confirm
	@echo 'Running up migrations'
	migrate -path=./migrations -database=${GREENLIGHT_DB_DSN} up

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

audit:
	@echo 'Tidying and verifying module dependencies...'
	go mod tidy
	go mod verify
	@echo 'Formatting code...'
	go fmt ./...
	@echo 'Vetting code...'
	go vet ./...
	staticcheck ./...
	@echo 'Running tests...'
	go test -race -vet=off ./...

# end
