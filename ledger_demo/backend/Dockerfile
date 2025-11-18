# build stage
FROM golang:1.25 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/walletapi ./ledger_demo/backend/cmd/walletapi

# runtime stage
FROM debian:12-slim
WORKDIR /srv
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/walletapi /srv/walletapi
ENV WALLETAPI_LISTEN_ADDR=:9090
USER root
CMD ["/srv/walletapi"]
