##
# Greenlight
#
# @file
# @version 0.1


.DEFAULT_GOAL := build

.PHONY:vet build

vet:
	go vet ./...

build: vet
	go build ./cmd/api/

run: vet
	go build ./cmd/api/ && ./api

clean:
	go clean -x

# end
