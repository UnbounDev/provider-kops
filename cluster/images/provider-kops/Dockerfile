
FROM debian:bookworm-slim

ARG TARGETOS
ARG TARGETARCH

ADD bin/$TARGETOS\_$TARGETARCH/provider /usr/local/bin/crossplane-kops-provider
ADD bin/kubectl /usr/local/bin/kubectl
ADD bin/kops /usr/local/bin/kops

RUN apt-get update \
    && apt-get install -yq bash procps ca-certificates \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

USER 65532
ENTRYPOINT ["crossplane-kops-provider"]

#FROM gcr.io/distroless/static@sha256:41972110a1c1a5c0b6adb283e8aa092c43c31f7c5d79b8656fbffff2c3e61f05