FROM scratch


COPY License.txt /License.txt
COPY README.txt /README.txt
COPY build/bin/main /usr/local/bin/event-logger

USER 8453

ENTRYPOINT ["/usr/local/bin/event-logger"]