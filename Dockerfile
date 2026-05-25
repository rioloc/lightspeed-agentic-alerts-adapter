FROM registry.access.redhat.com/ubi9/go-toolset:1.23 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o lightspeed-agentic-alerts-adapter ./cmd

FROM registry.access.redhat.com/ubi9-micro:latest

COPY --from=builder /build/lightspeed-agentic-alerts-adapter /usr/local/bin/lightspeed-agentic-alerts-adapter

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/lightspeed-agentic-alerts-adapter"]
