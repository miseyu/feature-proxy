GIT_VER := $(shell git describe --tags)
DATE := $(shell date +%Y-%m-%dT%H:%M:%S%z)

feature-proxy: *.go cmd/*.go go.mod go.sum
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$(GIT_VER) -X main.buildDate=$(DATE)" -o feature-proxy ./cmd/main.go

clean:
	rm -rf dist/* feature-proxy

run: feature-proxy
	./feature-proxy

packages:
	goreleaser release --rm-dist --snapshot --skip-publish

docker-image:
	docker build -t ghcr.io/miseyu/feature-proxy:$(GIT_VER) -f Dockerfile .

push-image: docker-image
	docker push ghcr.io/miseyu/feature-proxy:$(GIT_VER)

test:
	go test -v ./...

install:
	go install github.com/miseyu/feature-proxy/cmd/feature-proxy
