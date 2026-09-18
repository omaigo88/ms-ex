# Build context is the repo root (see deploy/compose/docker-compose.yaml):
# the go.work workspace ties order/inventory/payment/shared together via
# `replace` directives, so the whole repo is needed to build any one module.
FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY order/go.mod order/go.sum ./order/
COPY inventory/go.mod inventory/go.sum ./inventory/
COPY payment/go.mod payment/go.sum ./payment/
COPY shared/go.mod shared/go.sum ./shared/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/inventory ./inventory/cmd

FROM alpine:3.22
RUN adduser -D -u 10001 appuser
COPY --from=builder /out/inventory /usr/local/bin/inventory
USER appuser
EXPOSE 50051
ENTRYPOINT ["inventory"]
