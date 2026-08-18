# Build stage
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Runtime stage (distroless, non-root)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
WORKDIR /app
EXPOSE 8080
# No ENTRYPOINT: docker-compose selects the binary per service via `command:`
# (["/app/api"], ["/app/worker"], ["/app/migrate", "/app/migrations"]).
# An image ENTRYPOINT would override the compose command and run api everywhere.