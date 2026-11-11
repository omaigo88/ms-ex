---
paths:
  - "shared/pkg/kafka/**"
  - "*/pkg/kafka/**"
  - "shared/proto/events/**"
  - "assembly/**"
---

# Kafka

- Client: IBM/sarama, wrapped once in `shared/pkg/kafka` (`Producer`, `Consumer`,
  `Handler`, `DropMalformed`). Services don't use sarama directly outside their
  `pkg/kafka` adapters and `internal/app`.
- Topics are constants in `shared/pkg/kafka` (`TopicOrderPaid = "order.paid"`,
  `TopicShipAssembled = "ship.assembled"`) and are **created by the `kafka-init`
  compose service** — never rely on auto-create. A new topic needs both.
- Payloads are protobuf messages from `shared/proto/events/v1` (marshal with
  `proto.Marshal`). Every event carries `event_uuid`, `order_uuid`, `user_uuid`.
- Message key = `order_uuid`, so one order's events stay ordered on one partition.
- Delivery is **at-least-once**: the consumer marks the offset only when the
  handler returns nil; an error → the message (and its partition) is redelivered.
  Therefore every handler must be **idempotent** (e.g. `MarkAssembled` is a no-op
  for an already `ASSEMBLED` order).
- Handler error policy:
  - undecodable payload / invalid UUID → `sharedkafka.DropMalformed` (log, return nil)
    so a poison message can't block the partition;
  - transient failures (DB, gRPC) → return the error to get a retry;
  - business "can't apply" (unknown order, wrong status) → log and return nil.
- Messages are processed one at a time per partition; partitions run concurrently.
  Long work (Assembly's 5–15 s build) blocks its partition — scale with
  partitions + replicas in the same consumer group, not goroutines inside a handler.
- Consumer group IDs: `order-service`, `assembly-service`. Broker list from
  `KAFKA_BROKERS` (default `localhost:9094` for host runs, `kafka:9092` in compose).
- Publishing happens after the DB transaction commits; there is no outbox (known limitation).
