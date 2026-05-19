# syntax=docker/dockerfile:1

FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /musix ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 1000 musix
WORKDIR /app
COPY --from=build /musix .
COPY --from=web /web/dist ./web/dist
COPY config/config.docker.yaml.example ./config/config.yaml
RUN mkdir -p /app/data /app/downloads \
  && chown -R musix:musix /app
USER musix
EXPOSE 8080
ENV CONFIG_FILE=/app/config/config.yaml
CMD ["./musix"]
