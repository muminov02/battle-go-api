# Multi-stage: build both binaries, ship a tiny image. CMD selects api or worker.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/api    ./cmd/api
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.20
# ca-certificates: Ably HTTPS. tzdata: loc=Local time correctness (set TZ in env).
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
COPY --from=build /out/api    /app/api
COPY --from=build /out/worker /app/worker
USER app
EXPOSE 8080
CMD ["/app/api"]
