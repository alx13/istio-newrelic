FROM golang:1.14

WORKDIR /go/src/nr-tracing

COPY \
    go.mod \
    go.sum \
    main.go \
    ./

RUN set -x \
  && CGO_ENABLED=0 go build -o /go/bin/nr-app

CMD ["/go/bin/nr-app"]
