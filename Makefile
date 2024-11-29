SHELL = /bin/bash
.SHELLFLAGS = -o pipefail -c

.PHONY: help
help: ## Print info about all commands
	@echo "Commands:"
	@echo
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[01;32m%-20s\033[0m %s\n", $$1, $$2}'

#.PHONY: test
#test:
#	go test ./...

pgdata:
	./pginit.sh

pgdata/postmaster.pid: pgdata
	./pgstart.sh

.PHONY: serve
serve: pgdata/postmaster.pid
	source pg.env && go run . serve

.PHONY: clean
clean:
	./pgstop.sh
	rm hermeticum

.PHONY: connect
connect:
	go run . connect

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/hermeticum.proto
