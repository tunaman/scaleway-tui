BINARY_NAME=scw-tui

build:
	go mod tidy
	go build -o bin/$(BINARY_NAME) .

clean:
	rm -rf bin/
