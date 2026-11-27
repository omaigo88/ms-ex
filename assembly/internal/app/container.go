// Package app wires AssemblyService's dependency graph: the Kafka consumer
// and producer around the assembly business logic.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	assemblykafka "github.com/omaigo88/assembly/pkg/kafka"
	"github.com/omaigo88/assembly/pkg/service"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	"github.com/omaigo88/shared/pkg/logger"
	"github.com/omaigo88/shared/pkg/telemetry"
)

const (
	defaultKafkaBrokers  = "localhost:9094"
	defaultMinBuildDelay = 5 * time.Second
	defaultMaxBuildDelay = 15 * time.Second

	consumerGroupID = "assembly-service"
	serviceName     = "assembly-service"
)

// Container holds AssemblyService's constructed dependencies.
type Container struct {
	telemetry []telemetry.ShutdownFunc
	producer  *sharedkafka.Producer
	consumer  *sharedkafka.Consumer
	handler   sharedkafka.Handler
}

// New builds a Container connected to Kafka (KAFKA_BROKERS, defaulting to
// localhost:9094). Each assembly takes a random time between
// ASSEMBLY_BUILD_DELAY_MIN and ASSEMBLY_BUILD_DELAY_MAX (Go durations,
// default 5s and 15s).
func New(ctx context.Context) (_ *Container, err error) {
	// Logs only: the assembly path is outside the traced create-order flow.
	shutdowns, err := telemetry.Init(ctx, serviceName, logger.Init)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			telemetry.Flush(shutdowns...)
		}
	}()

	minBuildDelay, err := durationEnv("ASSEMBLY_BUILD_DELAY_MIN", defaultMinBuildDelay)
	if err != nil {
		return nil, err
	}

	maxBuildDelay, err := durationEnv("ASSEMBLY_BUILD_DELAY_MAX", defaultMaxBuildDelay)
	if err != nil {
		return nil, err
	}

	rawBrokers := os.Getenv("KAFKA_BROKERS")
	if rawBrokers == "" {
		rawBrokers = defaultKafkaBrokers
	}

	brokers := sharedkafka.ParseBrokers(rawBrokers)

	producer, err := sharedkafka.NewProducer(brokers)
	if err != nil {
		return nil, err
	}

	consumer, err := sharedkafka.NewConsumer(brokers, consumerGroupID, []string{sharedkafka.TopicOrderPaid})
	if err != nil {
		_ = producer.Close()

		return nil, err
	}

	return &Container{
		telemetry: shutdowns,
		producer:  producer,
		consumer:  consumer,
		handler:   assemblykafka.OrderPaidHandler(service.NewAssembler(minBuildDelay, maxBuildDelay), producer),
	}, nil
}

// Run consumes OrderPaid events until ctx is cancelled.
func (c *Container) Run(ctx context.Context) error {
	return c.consumer.Run(ctx, c.handler)
}

// Close releases the Container's resources.
func (c *Container) Close() {
	_ = c.consumer.Close()
	_ = c.producer.Close()
	telemetry.Flush(c.telemetry...)
}

// durationEnv reads a Go duration from the environment variable key, falling
// back to def when it is unset.
func durationEnv(key string, def time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return d, nil
}
