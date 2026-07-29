# syntax=docker/dockerfile:1

FROM docker.io/library/golang:1.24-alpine3.21 AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

WORKDIR /src
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/upgopher .
RUN mkdir -p /rootfs/data /rootfs/tmp \
    && chown -R 65532:65532 /rootfs \
    && chmod 1777 /rootfs/tmp

FROM scratch

LABEL org.opencontainers.image.title="Upgopher" \
      org.opencontainers.image.description="A self-contained browser file and clipboard sharing server" \
      org.opencontainers.image.source="https://github.com/wanetty/upgopher" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build --chown=65532:65532 /out/upgopher /upgopher
COPY --from=build --chown=65532:65532 /rootfs/data /data
COPY --from=build --chown=65532:65532 /rootfs/tmp /tmp

USER 65532:65532
WORKDIR /data
VOLUME ["/data"]
EXPOSE 9090

ENTRYPOINT ["/upgopher"]
CMD ["-dir", "/data"]
