.PHONY: build build-static clean

BINARY=loghook

build:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) main.go

build-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY) main.go

clean:
	rm -f $(BINARY)
