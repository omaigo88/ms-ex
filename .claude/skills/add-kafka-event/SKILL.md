---
name: add-kafka-event
description: Add a new Kafka event (topic, protobuf message, producer and consumer adapters, idempotent handling, compose topic creation, tests). Use when a task introduces async communication between services or a new event type.
---

# Add a Kafka event

Reference implementation: `OrderPaid` (Order → Assembly) and `ShipAssembled`
(Assembly → Order). Read `.claude/rules/kafka.md` first.

1. **Message** — `shared/proto/events/v1/events.proto`: new message with
   `event_uuid`, `order_uuid`, `user_uuid` + payload fields, all commented.
   `task proto:lint && task proto:gen`.
2. **Topic** — constant `TopicX = "domain.verb"` in `shared/pkg/kafka/kafka.go`
   **and** add it to the `kafka-init` command in `deploy/compose/docker-compose.yaml`
   (same partition count as existing topics).
3. **Producer side** (`<svc>`):
   - Consumer-owned publisher interface + domain-typed event struct in
     `pkg/service` (no proto/sarama there), plus a no-op implementation for tests.
   - Adapter in `<svc>/pkg/kafka` that builds the proto message, generates
     `event_uuid`, sends with key = `order_uuid` via the `Sender` interface.
   - Publish **after** the DB transaction commits.
4. **Consumer side** (`<svc>`):
   - Service method that is **idempotent** (re-applying the same event is a no-op)
     and runs in a transaction with `…ForUpdate`.
   - `XHandler(svc) sharedkafka.Handler` in `<svc>/pkg/kafka`: unmarshal →
     validate UUIDs (`DropMalformed` on failure) → call service → return its error.
   - Wire `sharedkafka.NewConsumer(brokers, "<svc>-service", []string{TopicX})`
     in `internal/app`, run it from `cmd/main.go` next to the server, close on shutdown.
   - Compose: the service gets `KAFKA_BROKERS: kafka:9092` and depends on `kafka-init`.
5. `.mockery.yaml` entry for any new `pkg/kafka` package; `task mocks:gen`.
6. **Tests**: adapter publishes correct message/key; handler happy path,
   service error is returned (retry), malformed message dropped; service
   idempotency on redelivery; API test calling the service method directly;
   e2e scenario that polls for the resulting state (with timeout).
7. Run the `verify` skill (e2e is mandatory here). Update the sequence diagram
   in `docs/architecture.md`.
