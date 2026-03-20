package main

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSClient wraps NATS JetStream for publishing and consuming.
type NATSClient struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

func NewNATSClient(url string) (*NATSClient, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}

	return &NATSClient{conn: conn, js: js}, nil
}

func (c *NATSClient) EnsureStream(ctx context.Context) error {
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "CONTRACTS",
		Subjects:  []string{"contracts.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("nats ensure CONTRACTS stream: %w", err)
	}
	return nil
}

func (c *NATSClient) EnsurePaymentsConsumer(ctx context.Context) (jetstream.Consumer, error) {
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, "PAYMENTS", jetstream.ConsumerConfig{
		Durable: "contracts-saga",
		FilterSubjects: []string{
			"payments.payment.held",
			"payments.payment.released",
			"payments.payment.transferred",
			"payments.payment.failed",
		},
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("nats ensure payments consumer: %w", err)
	}
	return consumer, nil
}

func (c *NATSClient) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := c.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}
	return nil
}

func (c *NATSClient) Close() {
	c.conn.Close()
}
