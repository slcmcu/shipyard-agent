all:
	@go get -d -v ./...
	@cd ./agent && go build -o ../shipyard-agent

fmt:
	@go fmt ./...
test:
	@go test ./...
clean:
	@rm -rf shipyard-agent
