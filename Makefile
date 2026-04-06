.PHONY: all build run tidy clean reset db

all: build

build:
	go build -o bin/server ./cmd/server
	go build -o bin/poller ./cmd/poller

tidy:
	go mod tidy

clean:
	rm -rf bin/

reset: clean
	rm -f vehicles.db
	rm -rf bin/
