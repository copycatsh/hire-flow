package main

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

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
		Name:      "JOBS",
		Subjects:  []string{"jobs.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("nats ensure stream: %w", err)
	}
	return nil
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
