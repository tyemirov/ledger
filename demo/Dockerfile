# syntax=docker/dockerfile:1
FROM golang:1.25 AS build

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /app
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend ./backend
RUN cd backend && go build -o /out/demo ./cmd/demo

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=build /out/demo /usr/local/bin/demo

ENV DEMOAPI_LISTEN_ADDR=:9090

ENTRYPOINT ["/usr/local/bin/demo"]
