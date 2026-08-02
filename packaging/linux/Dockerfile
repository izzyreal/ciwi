FROM golang:1.25.7-bookworm

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        git \
        libegl1-mesa-dev \
        libgles2-mesa-dev \
        libx11-dev \
        libx11-xcb-dev \
        libxcursor-dev \
        libxfixes-dev \
        libxkbcommon-dev \
        libxkbcommon-x11-dev \
        pkg-config \
        zip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work

