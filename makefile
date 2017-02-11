RMQ_IMG_NAME = rmq-trading
AMQP_PORT = 44430
MGMT_PORT = 8080

.PHONY: run start stop clean tail

run:
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

start:
	docker start ${RMQ_IMG_NAME}

stop:
	docker stop ${RMQ_IMG_NAME}

clean: stop
	docker rm ${RMQ_IMG_NAME}

tail:
	@docker logs -f ${RMQ_IMG_NAME}
