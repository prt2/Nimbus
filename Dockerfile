FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY k8s ./k8s

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/nimbus-api ./cmd/api

FROM alpine:3.22

RUN adduser -D -u 10001 nimbus

WORKDIR /app

COPY --from=builder /out/nimbus-api /usr/local/bin/nimbus-api

USER nimbus

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/nimbus-api"]
