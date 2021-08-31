.PHONY: config
config:
	rm -rf clickhouse01 clickhouse02 clickhouse03 clickhouse04
	mkdir -p clickhouse01 clickhouse02 clickhouse03 clickhouse04
	rm -rf clickhouse01_db clickhouse02_db clickhouse03_db clickhouse04_db
	mkdir -p clickhouse01_db clickhouse02_db clickhouse03_db clickhouse04_db
	rm -rf zookeeper
	mkdir -p zookeeper
	REPLICA=01 SHARD=01 envsubst < config.xml > clickhouse01/config.xml
	REPLICA=02 SHARD=01 envsubst < config.xml > clickhouse02/config.xml
	REPLICA=03 SHARD=02 envsubst < config.xml > clickhouse03/config.xml
	REPLICA=04 SHARD=02 envsubst < config.xml > clickhouse04/config.xml
	cp users.xml clickhouse01/users.xml
	cp users.xml clickhouse02/users.xml
	cp users.xml clickhouse03/users.xml
	cp users.xml clickhouse04/users.xml

.PHONY: up
up:
	docker-compose up -d

.PHONY: start
start:
	docker-compose start

.PHONY: down
down:
	docker-compose down
