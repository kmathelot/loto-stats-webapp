# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/loto-stats ./cmd/loto-stats

# distroless: no shell, runs as non-root (uid 65532)
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/loto-stats /app/loto-stats
# Default history, loaded at startup. In Kubernetes, DATA_DIR can point to a
# volume filled by an init container instead.
COPY --chmod=0755 data/ /app/data/
ENV LISTEN_ADDR=:8080 DATA_DIR=/app/data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/loto-stats"]
