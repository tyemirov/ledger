# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25 AS build

ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0

WORKDIR /app
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend ./backend
RUN cd backend && GOOS="${TARGETOS:-$(go env GOOS)}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -o /out/demo ./cmd/demo

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=build /out/demo /usr/local/bin/demo

ENV DEMOAPI_LISTEN_ADDR=:9090

ENTRYPOINT ["/usr/local/bin/demo"]
