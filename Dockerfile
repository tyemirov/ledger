# build stage
ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

ARG TARGETOS
ARG TARGETARCH
ARG GO_VERSION

WORKDIR /app
COPY go.mod go.sum ./
RUN expected_go_version="$(awk '/^go / { print $2; exit }' go.mod | cut -d. -f1,2)" \
	&& [ -n "${expected_go_version}" ] \
	&& [ "${expected_go_version}" = "${GO_VERSION}" ] \
	|| (echo "go.mod requires Go ${expected_go_version}, but Dockerfile GO_VERSION=${GO_VERSION}" >&2; exit 1)
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-$(go env GOOS)}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" go build -o /out/ledgerd ./cmd/credit

# runtime stage
FROM alpine:3.21
WORKDIR /srv
RUN apk add --no-cache ca-certificates
COPY --from=build /out/ledgerd /srv/ledgerd
USER root
CMD ["/srv/ledgerd"]
