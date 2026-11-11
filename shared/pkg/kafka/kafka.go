// Package kafka holds the small amount of sarama plumbing shared by the
// services that talk to Kafka: a protobuf-aware sync producer and a consumer
// group runner with at-least-once semantics.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"
)

// Topic names shared by producers and consumers.
const (
	TopicOrderPaid     = "order.paid"
	TopicShipAssembled = "ship.assembled"
)

// ParseBrokers splits a comma-separated broker list (e.g. the KAFKA_BROKERS
// environment variable).
func ParseBrokers(raw string) []string {
	var brokers []string

	for _, b := range strings.Split(raw, ",") {
		if b = strings.TrimSpace(b); b != "" {
			brokers = append(brokers, b)
		}
	}

	return brokers
}

// Producer publishes protobuf messages to Kafka synchronously.
type Producer struct {
	producer sarama.SyncProducer
}

// NewProducer connects a synchronous producer to brokers.
func NewProducer(brokers []string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Return.Successes = true

	p, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Producer{producer: p}, nil
}

// Send marshals msg and publishes it to topic under key. Messages sharing a key
// land on the same partition, so they're delivered in order.
func (p *Producer) Send(_ context.Context, topic, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal %s message: %w", topic, err)
	}

	_, _, err = p.producer.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(payload),
	})
	if err != nil {
		return fmt.Errorf("send %s message: %w", topic, err)
	}

	return nil
}

// Close flushes and closes the producer.
func (p *Producer) Close() error {
	return p.producer.Close()
}

// Handler processes one consumed message. Returning an error leaves the
// message's offset uncommitted, so it's redelivered.
type Handler func(ctx context.Context, msg *sarama.ConsumerMessage) error

// Consumer runs a consumer group over a set of topics.
type Consumer struct {
	group  sarama.ConsumerGroup
	topics []string
}

// NewConsumer joins groupID on brokers, consuming topics from the oldest
// uncommitted offset.
func NewConsumer(brokers []string, groupID string, topics []string) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer group %q: %w", groupID, err)
	}

	return &Consumer{group: group, topics: topics}, nil
}

// Run consumes until ctx is cancelled, dispatching every message to handle.
func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	gh := groupHandler{handle: handle}

	for {
		// Consume returns on every rebalance; loop to rejoin the group.
		if err := c.group.Consume(ctx, c.topics, gh); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}

			return fmt.Errorf("consume %v: %w", c.topics, err)
		}

		// Back off before rejoining so a message that keeps failing doesn't spin.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retryBackoff):
		}
	}
}

const retryBackoff = time.Second

// Close leaves the consumer group.
func (c *Consumer) Close() error {
	return c.group.Close()
}

// DropMalformed logs a message that can never be processed and acknowledges
// it (returns nil), so a poison message doesn't block its partition forever.
func DropMalformed(msg *sarama.ConsumerMessage, err error) error {
	slog.Error("dropping malformed kafka message",
		"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", err)

	return nil
}

type groupHandler struct {
	handle Handler
}

func (groupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (groupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h groupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			if err := h.handle(session.Context(), msg); err != nil {
				// Not marking the offset means this message (and the rest of the
				// partition) is redelivered on the next session.
				slog.Error("kafka message handling failed, will be redelivered",
					"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", err)

				return err
			}

			session.MarkMessage(msg, "")
		}
	}
}
