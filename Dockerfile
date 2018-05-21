FROM ubuntu:16.04

COPY bin/k8s-ghost-device-plugin /usr/local/bin/

ENTRYPOINT ["k8s-ghost-device-plugin"]
