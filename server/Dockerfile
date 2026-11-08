# Built with the server/ directory as the context (`make build` or `make release`).
# The Go stage runs on the build machine's platform and cross-compiles, so building
# linux/amd64 for Unraid on an Apple Silicon Mac doesn't emulate the compiler.
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY admin ./admin
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/riffle-server .

FROM debian:bookworm-slim
# Debian's ffmpeg loads the NVIDIA decoder (NVDEC) libraries at runtime, so with
# --runtime=nvidia the GPU is used for trickplay; without it, it falls back to CPU.
# intel-media-va-driver is the VAAPI driver for the Intel iGPU (amd64 only).
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && if [ "$(dpkg --print-architecture)" = amd64 ]; then \
      apt-get install -y --no-install-recommends intel-media-va-driver; fi \
 && rm -rf /var/lib/apt/lists/*

ENV RIFFLE_ADDR=:8770 \
    RIFFLE_MEDIA=/media \
    RIFFLE_DATA=/data \
    RIFFLE_HWACCEL=cuda \
    NVIDIA_DRIVER_CAPABILITIES=compute,video,utility

COPY --from=build /out/riffle-server /usr/local/bin/riffle-server

# Owned by the runtime user, so the default volume is writable when no /data is mounted.
RUN mkdir -p /data && chown 99:100 /data

EXPOSE 8770
VOLUME /data
# Unraid's nobody:users, so files in appdata keep the usual ownership.
USER 99:100
ENTRYPOINT ["riffle-server"]
