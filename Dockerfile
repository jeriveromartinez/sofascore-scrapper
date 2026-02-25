FROM golang:1.24-alpine AS builder

RUN apk add --no-cache chromium chromium-chromedriver

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o sofascore-scrapper .

FROM alpine:3.20

RUN apk add --no-cache chromium chromium-chromedriver ca-certificates

WORKDIR /app

COPY --from=builder /app/sofascore-scrapper .

CMD ["./sofascore-scrapper"]
