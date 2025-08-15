# build stage
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/creditd ./cmd/credit

# runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /srv
COPY --from=build /out/creditd /srv/creditd
ENV GRPC_LISTEN_ADDR=:7000
CMD ["/srv/creditd"]

