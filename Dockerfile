FROM cgr.dev/chainguard/go AS builder

ARG BINARY
WORKDIR /build
ENV CGO_ENABLED=0
ENV GOTOOLCHAIN=auto
COPY ./go.mod .
COPY ./go.sum .
COPY ./cmd/ ./cmd/
COPY ./internal/ ./internal/
RUN go build -o bin/proxy ./cmd/${BINARY}

FROM cgr.dev/chainguard/static
WORKDIR /app
COPY --from=builder /build/bin/proxy .
ENTRYPOINT ["/app/proxy"]
