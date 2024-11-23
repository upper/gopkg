FROM golang:1.23-alpine3.20 AS builder

RUN mkdir -p /go/src/github.com/xiam/vanity

WORKDIR /go/src/github.com/xiam/vanity

COPY . .

RUN go build -o /go/bin/vanity github.com/xiam/vanity

FROM alpine:3.20

RUN apk add --no-cache \
  ca-certificates && update-ca-certificates

COPY --from=builder /go/bin/vanity /bin/

EXPOSE 9001

ENTRYPOINT ["/bin/vanity"]
