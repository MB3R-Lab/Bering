FROM golang:1.23 AS builder
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bering ./cmd/bering

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/bering /usr/local/bin/bering
ENTRYPOINT ["/usr/local/bin/bering"]

