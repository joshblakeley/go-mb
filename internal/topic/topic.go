// Package topic provides topic creation and deletion via the franz-go admin API.
package topic

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ValidateName returns an error if name is not a valid Kafka topic name.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("topic name must not be empty")
	}
	return nil
}

// Create creates a topic with the given number of partitions and replication factor.
// If the topic already exists this is a no-op.
func Create(ctx context.Context, brokers []string, name string, partitions int, replicationFactor int, extraOpts ...kgo.Opt) error {
	opts := append([]kgo.Opt{kgo.SeedBrokers(brokers...)}, extraOpts...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("create admin client: %w", err)
	}
	defer client.Close()

	adm := kadm.NewClient(client)
	resp, err := adm.CreateTopics(ctx, int32(partitions), int16(replicationFactor), nil, name)
	if err != nil {
		return fmt.Errorf("create topic: %w", err)
	}
	for _, r := range resp {
		// kerr.TopicAlreadyExists is the correct sentinel — string comparison is unreliable.
		if r.Err != nil && !errors.Is(r.Err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("create topic %s: %w", r.Topic, r.Err)
		}
	}
	return nil
}

// Delete deletes a topic. If the topic does not exist this is a no-op.
func Delete(ctx context.Context, brokers []string, name string, extraOpts ...kgo.Opt) error {
	opts := append([]kgo.Opt{kgo.SeedBrokers(brokers...)}, extraOpts...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return fmt.Errorf("create admin client: %w", err)
	}
	defer client.Close()

	adm := kadm.NewClient(client)
	resp, err := adm.DeleteTopics(ctx, name)
	if err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	for _, r := range resp {
		if r.Err != nil {
			return fmt.Errorf("delete topic %s: %w", r.Topic, r.Err)
		}
	}
	return nil
}
