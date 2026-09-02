# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata wget \
    && addgroup -S app && adduser -S -G app app
COPY --from=builder /out/api /usr/local/bin/api

# Ticket K2: uploaded audio files live here.
#
# Both lines matter. The image has no WORKDIR, so the config default of
# ./uploads resolves to /uploads — which `app` cannot create, and the process
# refuses to start rather than run without a place to put uploads. Pointing
# STORAGE_PATH at a directory that exists and is owned before the USER switch
# removes the whole class of problem.
ENV STORAGE_PATH=/var/lib/streampulse/uploads
RUN mkdir -p "$STORAGE_PATH" && chown -R app:app /var/lib/streampulse

USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/api"]
