# build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY . .
WORKDIR /app/demo/backend
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/demoapi ./cmd/demoapi

# runtime stage
FROM alpine:3.20
WORKDIR /srv
RUN apk add --no-cache ca-certificates
COPY --from=build /out/demoapi /srv/demoapi
ENV DEMOAPI_LISTEN_ADDR=:9090
EXPOSE 9090
USER root
CMD ["/srv/demoapi"]
