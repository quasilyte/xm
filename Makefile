GOPATH_DIR=`go env GOPATH`

.PHONY: test
test:
	go test -count 2 -v -race . ./internal/...

.PHONY: lint
lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH_DIR)/bin v1.56.2
	$(GOPATH_DIR)/bin/golangci-lint run . ./internal/...
	@echo "everything is OK"
