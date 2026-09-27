# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -trimpath -o /app/vrcmemes-bot .

FROM builder AS dev
RUN go install github.com/air-verse/air@v1.67.3

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS final

RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder --chown=app:app /app/vrcmemes-bot /app/vrcmemes-bot
USER app
CMD ["/app/vrcmemes-bot"]
