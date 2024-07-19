FROM golang:1.22 AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o /entrypoint

FROM gcr.io/distroless/static-debian12

COPY --from=builder /entrypoint /entrypoint 

EXPOSE 8080
ENTRYPOINT ["/entrypoint"]
