RMQ_IMG_NAME = rmq-trading
AMQP_PORT = 44430
MGMT_PORT = 8080

REDIS_IMG_NAME = redis-trading
REDIS_PORT = 44431
REDIS_KEY_PREFIX = quotemgr
REDIS_HOST = localhost
REDIS_CLI = redis-cli -h ${REDIS_HOST} -p ${REDIS_PORT}

.PHONY: install run start stop clean tail

install:
	go get
	go install
	chmod +x githooks/pre-push
	ln -s ${PWD}/githooks/pre-push ${PWD}/.git/hooks/pre-push

run: run-rmq run-redis

run-rmq:
	@docker run \
		--hostname ${RMQ_IMG_NAME} \
		--name ${RMQ_IMG_NAME} \
		--publish ${AMQP_PORT}:5672 \
		--publish ${MGMT_PORT}:15672 \
		--detach \
		rabbitmq:3.6.6-management

	@echo "${RMQ_IMG_NAME} is running on `docker inspect --format '{{ .NetworkSettings.IPAddress }}' ${RMQ_IMG_NAME}`"

	@echo "AMPQ: ${AMQP_PORT}"

	@echo "Management GUI: ${MGMT_PORT}"

run-redis:
	@docker run \
		--name ${REDIS_IMG_NAME} \
		--publish ${REDIS_PORT}:6379 \
		--detach \
		redis:3.2.7

	@echo "Redis: ${REDIS_PORT}"

start: start-rmq start-redis

start-rmq:
	docker start ${RMQ_IMG_NAME}
	@echo "AMPQ: ${AMQP_PORT}"
	@echo "Management GUI: ${MGMT_PORT}"

start-redis:
	docker start ${REDIS_IMG_NAME}
	@echo "Redis: ${REDIS_PORT}"

stop: stop-rmq stop-redis

stop-rmq:
	docker stop ${RMQ_IMG_NAME}

stop-redis:
	docker stop ${REDIS_IMG_NAME}

redis-remove-keys:
	@${REDIS_CLI} --scan --pattern '${REDIS_KEY_PREFIX}:*' | xargs ${REDIS_CLI} del

clean: clean-rmq clean-redis

clean-rmq: stop-rmq
	docker rm ${RMQ_IMG_NAME}

clean-redis: stop-redis
	docker rm ${REDIS_IMG_NAME}

tail:
	@docker logs -f ${RMQ_IMG_NAME}
