FROM golang:1.22 AS builder
WORKDIR /app

COPY go.* ./
RUN go mod download
COPY *.go ./
RUN go build -o /server

FROM debian:12-slim
WORKDIR /app

COPY --from=builder /server /server 

EXPOSE 8080
ENTRYPOINT ["/server"]
