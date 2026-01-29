.PHONY: build run clean

build:
	go build -o bin/control-plane main.go

dev:
	go run main.go

run: build
	JWT_SECRET=secret123 ./bin/control-plane -l localhost:9900

clean:
	rm -rf bin/
