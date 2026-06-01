
version := 0.1.0

help:
	@echo "Available targets:"
	@echo "  docker-build - Build the Docker image for the flight price tracker application."

container-build:
	podman build -t flight-price-tracker:$(version) .
