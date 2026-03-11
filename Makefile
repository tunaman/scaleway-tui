BINARY_NAME=scw-tui

build:
	go mod tidy
	go build -o bin/$(BINARY_NAME) main.go

clean:
	rm -rf bin/
