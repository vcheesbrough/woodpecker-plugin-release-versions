FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG RELEASE_TAG=
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags="-s -w -X github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/version.ReleaseTag=${RELEASE_TAG}" \
    -o /plugin .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates git

COPY --from=builder /plugin /plugin

ENTRYPOINT ["/plugin"]
