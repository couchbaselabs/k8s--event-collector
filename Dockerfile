FROM scratch

ARG TARGETARCH
COPY License.txt /License.txt
COPY build/bin/linux/k8s-event-collector-${TARGETARCH} /usr/local/bin/k8s-event-collector

USER 8453

ENTRYPOINT ["/usr/local/bin/k8s-event-collector"]