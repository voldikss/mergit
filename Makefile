dev:
	make build && make run

run:
	./mergit

build:
	go build

clean:
	rm -rf ./mergit || true

test:
	go test -v ./...
