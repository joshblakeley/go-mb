.PHONY: build test test-integration broker-up broker-down broker-wait e2e

# Build the bench binary
build:
	go build -o bench ./cmd/bench

# Run unit tests only
test:
	go test ./...

# Start Redpanda broker (detached)
broker-up:
	docker compose up -d
	@echo "Waiting for broker to be healthy..."
	@docker compose wait redpanda 2>/dev/null || \
		(sleep 5 && echo "Broker should be up. Check: docker compose ps")

# Stop and remove broker
broker-down:
	docker compose down -v

# Run integration tests (requires broker)
test-integration:
	BENCH_BROKERS=localhost:9092 go test ./internal/bench/ \
		-tags integration -v -run TestIntegration -timeout 90s

# Full E2E: start broker, run integration tests, stop broker
e2e: build broker-up
	@echo ""
	@echo "=== Running integration smoke test ==="
	$(MAKE) test-integration
	@echo ""
	@echo "=== Running CLI end-to-end ==="
	./bench run \
		--brokers localhost:9092 \
		--producers 2 \
		--consumers 2 \
		--partitions 4 \
		--duration 20s \
		--warmup 3s \
		--report-interval 5s \
		--topic e2e-test-topic \
		--create-topic \
		--delete-topic
