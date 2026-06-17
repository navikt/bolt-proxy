.PHONY: all outbound inbound test vet

all: outbound inbound

outbound:
	go build -o bin/outbound ./cmd/outbound

inbound:
	go build -o bin/inbound ./cmd/inbound

test:
	go test ./...

vet:
	go vet ./...
