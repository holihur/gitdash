# Demo image for the gitdash OCI / Docker registry.
#
#   docker build -t hello:1.0 .
#   docker tag hello:1.0 your-host:8080/<owner>/hello:1.0
#   docker push your-host:8080/<owner>/hello:1.0
FROM alpine:3.19
COPY hello.sh /hello.sh
ENTRYPOINT ["/bin/sh", "/hello.sh"]
