FROM debian:jessie

RUN apt-get update

RUN apt-get install -y curl

# COPY bin/vanity_linux_amd64.gz /tmp
# RUN gzip -d /tmp/vanity_linux_amd64.gz
# RUN mv /tmp/vanity_linux_amd64 /bin/vanity

ENV VANITY_URL https://github.com/xiam/vanity/releases/download/v0.1.2/vanity_linux_amd64.gz
RUN curl --silent -L ${VANITY_URL} | gzip -d > /bin/vanity

RUN chmod +x /bin/vanity

RUN mkdir -p /var/run/vanity

EXPOSE 8080

ENTRYPOINT ["/bin/vanity"]
