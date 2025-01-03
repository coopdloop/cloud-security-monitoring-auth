// internal/aws/sns.go

package aws

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/rs/zerolog/log"
)

type SNSClient struct {
	client      *sns.Client
	topicARN    string
	isFifoTopic bool
}

func NewSNSClient(ctx context.Context) (*SNSClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	topicARN := os.Getenv("AWS_SNS_TOPIC_ARN")
	// Check if topic is FIFO by looking for .fifo suffix
	isFifoTopic := strings.HasSuffix(topicARN, ".fifo")

	return &SNSClient{
		client:      sns.NewFromConfig(cfg),
		topicARN:    topicARN,
		isFifoTopic: isFifoTopic,
	}, nil
}

// generateMessageDeduplicationId creates a unique deduplication ID for FIFO topics
func generateMessageDeduplicationId(eventType string, data interface{}) string {
	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(append([]byte(eventType), jsonData...))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes of hash
}

// generateMessageGroupId creates a group ID based on event type
func generateMessageGroupId(eventType string) string {
	parts := strings.Split(eventType, ".")
	return parts[0] // Use the first part of the event type as group ID
}

// PublishEvent sends an event to SNS
func (s *SNSClient) PublishEvent(ctx context.Context, eventType string, data interface{}) error {
	payload := struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}{
		Type: eventType,
		Data: data,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	jsonPayloadString := string(jsonPayload)

	input := &sns.PublishInput{
		Message:  &jsonPayloadString,
		TopicArn: &s.topicARN,
	}

	// Add FIFO-specific attributes if topic is FIFO
	if s.isFifoTopic {
		deduplicationId := generateMessageDeduplicationId(eventType, data)
		groupId := generateMessageGroupId(eventType)

		input.MessageDeduplicationId = &deduplicationId
		input.MessageGroupId = &groupId
	}

	// Add message attributes for filtering
	input.MessageAttributes = map[string]types.MessageAttributeValue{
		"event_type": {
			DataType:    aws.String("String"),
			StringValue: &eventType,
		},
	}

	_, err = s.client.Publish(ctx, input)
	if err != nil {
		log.Error().Err(err).
			Str("event_type", eventType).
			Interface("data", data).
			Msg("Failed to publish event to SNS")
		return err
	}

	log.Info().
		Str("event_type", eventType).
		Interface("data", data).
		Msg("Successfully published event to SNS")

	return nil
}
