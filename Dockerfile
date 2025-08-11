# Use the $BUILDPLATFORM for the build stage
FROM --platform=$BUILDPLATFORM golang:1.24.6-alpine3.21 AS builder

WORKDIR /app

# Prepare dependencies
COPY go.mod go.sum .
RUN go mod download

# Copy the sources and config
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./config ./config

# Build the binary based on the target platform
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o api ./cmd/api

# Use the $TARGETPLATFORM by default for the runtime stage
FROM alpine:3.22
WORKDIR /app
COPY --from=builder /app/api ./api
COPY --from=builder /app/config ./config
EXPOSE 8080
CMD ["./api"]