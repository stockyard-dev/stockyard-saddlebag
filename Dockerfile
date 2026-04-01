FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/saddlebag ./cmd/saddlebag/
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata curl
COPY --from=builder /bin/saddlebag /usr/local/bin/saddlebag
ENV PORT="8970" DATA_DIR="/data"
EXPOSE 8970
HEALTHCHECK --interval=30s --timeout=5s CMD curl -sf http://localhost:8970/health || exit 1
ENTRYPOINT ["saddlebag"]
