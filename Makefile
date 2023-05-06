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

tag := $(shell git tag | head -n1)

build-image:
	docker build -t mergit:$(tag) .
	docker build -t mergit:latest .

push-image:
	docker push mergit:$(tag)
	docker push mergit:latest
