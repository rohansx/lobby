# The UI and the Go binary are built on the build host and cross-compiled,
# so a multi-arch image needs no emulation.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npx vite build --outDir /dist

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /dist ./internal/app/web/dist
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/lobby ./cmd/lobby

FROM litestream/litestream:0.5 AS litestream

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 lobby && mkdir /data && chown lobby /data
COPY --from=build /out/lobby /usr/local/bin/lobby
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY deploy/litestream.yml /etc/litestream.yml
COPY --chmod=755 deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN ln -s /usr/local/bin/lobby /usr/local/bin/wl
USER lobby
ENV LOBBY_DATA=/data LOBBY_ADDR=:8080
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --start-interval=1s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
