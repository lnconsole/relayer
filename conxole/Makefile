conxole: $(shell find .. -name "*.go")
	CC=$$(which musl-gcc) go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o ./conxole

server-dev:
	go run main.go

server-prod:
	go build -tags prod -o prod