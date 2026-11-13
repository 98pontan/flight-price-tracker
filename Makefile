version := 0.2.0
image   := flight-price-tracker
GO      ?= go

.PHONY: help build run run-demo test openapi container-build container-run clean

help:
	@echo "Available targets:"
	@echo "  build           - Build the server binary into bin/"
	@echo "  run             - Run the server locally with config.yaml"
	@echo "  run-demo        - Run with the demo search provider (made-up prices, no API key)"
	@echo "  test            - Run vet and tests"
	@echo "  openapi         - Write the generated OpenAPI spec to openapi.json"
	@echo "  container-build - Build the container image"
	@echo "  container-run   - Run the container (port 8080, data in volume flight-data)"

build:
	$(GO) build -ldflags "-X main.version=$(version)" -o bin/flight-tracker ./cmd/flight-tracker

run: build
	./bin/flight-tracker -config config.yaml

run-demo: build
	SEARCH_PROVIDER=demo ./bin/flight-tracker -config config.yaml

test:
	$(GO) vet ./...
	$(GO) test ./...

openapi: build
	./bin/flight-tracker -config config.yaml -openapi > openapi.json

container-build:
	podman build --build-arg VERSION=$(version) -t $(image):$(version) .

container-run: container-build
	podman run -d --name $(image) --replace -p 8080:8080 -v flight-data:/data \
		-e SERPAPI_KEY $(image):$(version)

clean:
	rm -rf bin
