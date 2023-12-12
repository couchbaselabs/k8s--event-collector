FROM scratch

COPY License.txt /License.txt
COPY build/bin/main /usr/local/bin/event-collector

USER 8453

ENTRYPOINT ["/usr/local/bin/event-collector"]