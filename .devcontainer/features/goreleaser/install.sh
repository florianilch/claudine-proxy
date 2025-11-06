#!/usr/bin/env bash

GORELEASER_VERSION=${VERSION:-"latest"}

echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update

if [ "$GORELEASER_VERSION" = "latest" ]; then
    sudo apt install -y goreleaser
else
    sudo apt install -y goreleaser=${GORELEASER_VERSION}
fi

sudo apt clean
rm /etc/apt/sources.list.d/goreleaser.list
rm -rf /var/lib/apt/lists/*
