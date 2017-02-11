Quote manager
====
[![Build Status](https://travis-ci.org/DistributedDesigns/quote_manager.svg?branch=master)](https://travis-ci.org/DistributedDesigns/quote_manager)

Service that handles interactions with the legacy quote server. Interaction takes place with RabbitMQ queues. Accepts incoming requests for quotes and broadcasts updates.

## Installing
```sh
git clone https://github.com/DistributedDesigns/quote_manager.git
make install

# start Docker to host RMQ, Redis containers
make run

# You can call either of the next two lines
$GOPATH/bin/quote_manager
go run *.go # *.go since `package main` is spread over multiple files
```

## Odds and ends
### Specifying a config
Call the service with the `--config=./path/to/config.yaml` flag and point it at a `.yaml` file following the schema in [./config](./config). The [dev config](./config/dev.yaml) is used by default.

### Docker startup / teardown
0. `make run` Creates new rabbit and redis containers
0. `make stop` Pauses RMQ, redis containers and saves state
0. `make start` Starts previously stopped RMQ, redis
0. `make clean` Deletes RMQ, redis containers. `make run` to recreate them.

### Redis namespace and key removal
All redis keys are prefixed with `quotemgr:` to separate them from other keys in the environment where one redis instance is shared between many services. You can call `make redis-remove-keys` to delete all keys `quotemgr:*` in the event you want the Quote Manager to start with a fresh cache without affecting other redis customers.

### Installing the linter
[metalinter][metalinter] will be run by CI. You can run the linter locally to check for problems early.

```shell
go get -u github.com/alecthomas/gometalinter

# Install all known linters
$GOPATH/bin/gometalinter --install

# Run the linter on all files the project
$GOPATH/bin/gometalinter --config=.gometalinterrc ./...
```

`make install` Will create a git hook that rejects pushes that fail the linter.

[metalinter]: https://github.com/alecthomas/gometalinter
