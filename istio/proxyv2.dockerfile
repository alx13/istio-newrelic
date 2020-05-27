FROM docker.io/istio/proxyv2:1.6.0

COPY \
  ./envoy_bootstrap_v2.json \
  /var/lib/istio/envoy/envoy_bootstrap_tmpl.json
