# ---- build ----
FROM docker.io/library/golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN go vet ./... && go test ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/flight-tracker ./cmd/flight-tracker \
 && mkdir -p /out/data

# ---- run ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/flight-tracker /app/flight-tracker
COPY config.yaml /app/config.yaml
COPY --from=build --chown=65532:65532 /out/data /data

ENV CONFIG_FILE=/app/config.yaml \
    DATABASE_PATH=/data/flights.db
VOLUME /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/flight-tracker"]
