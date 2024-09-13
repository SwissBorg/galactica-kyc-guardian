FROM golang:1.22.5-alpine3.19 AS builder
WORKDIR /app
RUN cat /etc/resolv.conf && apk add --no-cache git && \
	git config --add --global url."git@github.com:".insteadOf https://github.com

# Prepare dependencies
COPY go.mod go.sum ./
RUN go mod download
# Build the binary
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./config ./config
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api ./cmd/api


FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/api ./api
COPY --from=builder /app/config ./config
EXPOSE 8080
CMD ["./api"]