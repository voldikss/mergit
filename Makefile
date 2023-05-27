export LOGGING_LEVEL := DEBUG

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
	docker build -t voldikss/mergit:$(tag) .
	docker build -t voldikss/mergit:latest .

push-image:
	docker push voldikss/mergit:$(tag)
	docker push voldikss/mergit:latest
