# build stage
FROM golang:1.25 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/ledgerd ./cmd/credit

# runtime stage
FROM debian:12-slim
WORKDIR /srv
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ledgerd /srv/ledgerd
ENV GRPC_LISTEN_ADDR=:7000
USER root
CMD ["/srv/ledgerd"]
