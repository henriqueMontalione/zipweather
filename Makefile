.PHONY: run build test test-coverage lint docker-build docker-run clean

run:
	go run ./cmd/server

build:
	go build -o zipweather ./cmd/server

test:
	go test -race ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	go vet ./...

docker-build:
	docker build -t zipweather .

docker-run:
	docker run --rm -p 8080:8080 --env-file .env zipweather

clean:
	rm -f zipweather coverage.out
