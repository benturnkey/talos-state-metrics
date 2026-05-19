# Build stage
# TODO: switch back to stagex/pallet-go once it ships Go 1.26.1+
# (talos/pkg/machinery requires go 1.26.1, stagex pallet-go is currently at 1.26.0)
FROM golang:1.26.2-alpine@sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o talos-state-metrics ./cmd/talos-state-metrics

# Final image
FROM scratch

COPY --from=builder /app/talos-state-metrics /talos-state-metrics

ENTRYPOINT ["/talos-state-metrics"]
