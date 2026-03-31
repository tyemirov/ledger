# build stage
FROM --platform=$BUILDPLATFORM golang:1.25 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-$(go env GOOS)}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -o /out/ledgerd ./cmd/credit

# runtime stage
FROM alpine:3.21
WORKDIR /srv
RUN apk add --no-cache ca-certificates
COPY --from=build /out/ledgerd /srv/ledgerd
ENV GRPC_LISTEN_ADDR=:50051
USER root
CMD ["/srv/ledgerd"]
