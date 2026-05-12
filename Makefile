.PHONY: all build web clean run dev test

APP_NAME = cc-go
WEB_DIR = web

all: web build

web:
	cd $(WEB_DIR) && npm install && npm run build

build:
	go build -o $(APP_NAME) ./cmd/cc-go/

build-linux:
	cd $(WEB_DIR) && npm run build
	GOOS=linux GOARCH=amd64 go build -o $(APP_NAME)-linux ./cmd/cc-go/

build-mac:
	cd $(WEB_DIR) && npm run build
	GOOS=darwin GOARCH=amd64 go build -o $(APP_NAME)-mac ./cmd/cc-go/

build-win:
	cd $(WEB_DIR) && npm run build
	GOOS=windows GOARCH=amd64 go build -o $(APP_NAME).exe ./cmd/cc-go/

run:
	go run ./cmd/cc-go/

dev:
	cd $(WEB_DIR) && npm run dev &
	go run ./cmd/cc-go/

clean:
	rm -rf cmd/cc-go/web-dist $(APP_NAME) $(APP_NAME)-linux $(APP_NAME)-mac $(APP_NAME).exe

test:
	go test ./... -v -count=1 -timeout 60s