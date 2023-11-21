BUILDDIR := build/bin

VERSION ?= 0.1.0

.PHONY: bins image
all: | $(BUILDDIR)
	go build -o build/bin/main cmd/event-logger/main.go

.PHONY:
image: bins
	docker build -t couchbase/event-logger:$(VERSION) .


.PHONY: bins
bins: | $(BUILDDIR)
	go build -o build/bin/main cmd/event-logger/main.go

compile:
	echo "Compiling for every OS and Platform"
	GOOS=linux GOARCH=arm go build -o bin/main-linux-arm main.go
	GOOS=linux GOARCH=arm64 go build -o bin/main-linux-arm64 main.go
	GOOS=freebsd GOARCH=386 go build -o bin/main-freebsd-386 main.go

$(BUILDDIR):
	mkdir -p $(BUILDDIR)