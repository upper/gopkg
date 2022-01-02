FROM golang:1.17-bullseye AS builder

RUN mkdir -p /go/src/github.com/xiam/vanity
WORKDIR /go/src/github.com/xiam/vanity

COPY . .

RUN go build -o /go/bin/vanity .

FROM debian:bullseye

RUN apt-get update && \
  apt-get install -y ca-certificates file --no-install-recommends

COPY --from=builder /go/bin/vanity /bin/

EXPOSE 9001
ENTRYPOINT ["/bin/vanity"]
