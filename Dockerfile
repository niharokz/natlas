# ── build ────────────────────────────────────────────────────────────────
# Dependencies are vendored (vendor/), so the build needs no network access.
FROM mirror.gcr.io/library/golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/natlas ./cmd/natlas

# ── run ──────────────────────────────────────────────────────────────────
# An empty image: just the binary (web files and timezone data are built in)
# and the default plugins. About 10 MB.
FROM scratch
COPY --from=build /out/natlas /natlas
COPY plugins /app/plugins
ENV NATLAS_ADDR=:8080 \
    NATLAS_CONFIG=/config/natlas.yml \
    NATLAS_PLUGINS_DIR=/app/plugins
EXPOSE 8080
USER 1000:1000
HEALTHCHECK --interval=1m --timeout=5s --start-period=5s CMD ["/natlas", "health"]
ENTRYPOINT ["/natlas"]
