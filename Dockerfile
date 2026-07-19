FROM node:22-alpine AS build-vue

WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build-go

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/sofascore-scrapper ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=build-go /out/sofascore-scrapper .
COPY --from=build-vue /app/web/dist ./web/dist
COPY migrations/ ./migrations/

RUN chown -R appuser:appgroup /app

USER appuser

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health/live || exit 1

CMD ["./sofascore-scrapper"]
