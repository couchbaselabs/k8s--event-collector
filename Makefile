BUILDDIR := build/bin

VERSION ?= 0.1.0

.PHONY: all
all: bins images

.PHONY:
images: bins
	docker buildx build --platform linux/arm64 --build-arg TARGET=kubernetes-linux-arm64 --load --build-arg GO_VERSION=1.20.8 --build-arg VERSION=2.6.0 --build-arg BLD_NUM=999 -t couchbase/event-collector:$(VERSION) .


.PHONY: bins
bins: | $(BUILDDIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o build/bin/main cmd/event-collector/main.go

.PHONY: kind-images
kind-images: images
	kind load docker-image couchbase/event-collector:$(VERSION)

$(BUILDDIR):
	mkdir -p $(BUILDDIR)