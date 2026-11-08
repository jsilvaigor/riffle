FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/riffle-server .

FROM debian:bookworm-slim
# Debian's ffmpeg loads the NVIDIA decoder (NVDEC) libraries at runtime, so with
# --runtime=nvidia the GPU is used for trickplay; without it, it falls back to CPU.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && rm -rf /var/lib/apt/lists/*

ENV RIFFLE_ADDR=:8770 \
    RIFFLE_MEDIA=/media \
    RIFFLE_DATA=/data \
    RIFFLE_HWACCEL=cuda \
    NVIDIA_DRIVER_CAPABILITIES=compute,video,utility

COPY --from=build /out/riffle-server /usr/local/bin/riffle-server

EXPOSE 8770
VOLUME /data
# Unraid's nobody:users, so files in appdata keep the usual ownership.
USER 99:100
ENTRYPOINT ["riffle-server"]
