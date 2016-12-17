FROM debian:jessie

RUN apt-get update

RUN apt-get install -y curl

ENV VANITY_VERSION 0.1.3

ENV VANITY_URL https://github.com/xiam/vanity/releases/download/v${VANITY_VERSION}/vanity_linux_amd64.gz

RUN curl --silent -L ${VANITY_URL} | gzip -d > /bin/vanity

RUN chmod +x /bin/vanity

RUN mkdir -p /var/run/vanity

EXPOSE 8080

ENTRYPOINT ["/bin/vanity"]
