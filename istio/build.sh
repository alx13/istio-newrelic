#!/bin/bash

docker build -t alx13/proxyv2:1.6.0 -f proxyv2.dockerfile .
docker push alx13/proxyv2:1.6.0

docker pull docker.io/istio/pilot:1.6.0
docker tag docker.io/istio/pilot:1.6.0 alx13/pilot:1.6.0
docker push alx13/pilot:1.6.0