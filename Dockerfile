# Build the Go API in one stage, then copy only its executable into a small runtime image.

FROM golang:1.25-alpine AS builder

WORKDIR /src

# Copy dependency files first so Docker can reuse this layer when only source code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY k8s ./k8s

# Produce a Linux executable that does not need C libraries.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/nimbus-api ./cmd/api

# Start the runtime stage and run the executable as a dedicated user.
FROM alpine:3.22

RUN adduser -D -u 10001 nimbus

WORKDIR /app

COPY --from=builder /out/nimbus-api /usr/local/bin/nimbus-api

USER nimbus

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/nimbus-api"]
