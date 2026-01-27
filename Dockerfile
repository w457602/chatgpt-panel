FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/chatgpt-panel ./cmd

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /bin/chatgpt-panel /app/chatgpt-panel
COPY templates /app/templates
COPY static /app/static

ENV GIN_MODE=release

EXPOSE 8080

CMD ["/app/chatgpt-panel"]
