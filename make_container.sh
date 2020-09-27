#!/bin/sh
container=$(buildah from prom/alertmanager)
mnt=$(buildah mount $container)
CGO_ENABLED=0 go build -ldflags="-X 'main.Version=benny'" -o alertmanager cmd/alertmanager/main.go 
cp -f ./alertmanager "${mnt}/bin/alertmanager"
buildah commit $container ben-am
buildah unmount $container