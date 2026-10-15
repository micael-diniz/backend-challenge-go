package sqs

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
)

const (
	WagerQueue        = "wager-transactions.fifo"
	WagerDLQ          = "wager-transactions-dlq.fifo"
	EventsQueue       = "wallet-events.fifo"
	visibilityTimeout = 30
	maxReceiveCount   = 5
	waitTimeSeconds   = 1
	localStackRegion  = "us-east-1"
	localStackAccess  = "test"
	localStackSecret  = "test"
)

type API struct {
	client *sqs.Client
}

func NewAPI(endpoint string) *API {
	return &API{client: sqs.New(sqs.Options{
		Region:       localStackRegion,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(localStackAccess, localStackSecret, ""),
	})}
}

type Queue struct {
	client *sqs.Client
	url    string
}

func (api *API) Open(ctx context.Context) (*Queue, *Queue, error) {
	deadLetterURL, err := api.ensureFIFO(ctx, WagerDLQ, nil)
	if err != nil {
		return nil, nil, err
	}
	arn, err := api.queueARN(ctx, deadLetterURL)
	if err != nil {
		return nil, nil, err
	}
	wagerURL, err := api.ensureFIFO(ctx, WagerQueue, map[string]string{
		"RedrivePolicy":     fmt.Sprintf(`{"deadLetterTargetArn":%q,"maxReceiveCount":"%d"}`, arn, maxReceiveCount),
		"VisibilityTimeout": strconv.Itoa(visibilityTimeout),
	})
	if err != nil {
		return nil, nil, err
	}
	eventsURL, err := api.ensureFIFO(ctx, EventsQueue, map[string]string{
		"VisibilityTimeout": strconv.Itoa(visibilityTimeout),
	})
	if err != nil {
		return nil, nil, err
	}
	return &Queue{client: api.client, url: wagerURL}, &Queue{client: api.client, url: eventsURL}, nil
}

func (api *API) deadLetter(ctx context.Context) (*Queue, error) {
	url, err := api.ensureFIFO(ctx, WagerDLQ, nil)
	if err != nil {
		return nil, err
	}
	return &Queue{client: api.client, url: url}, nil
}

func (api *API) ensureFIFO(ctx context.Context, name string, extra map[string]string) (string, error) {
	attributes := map[string]string{
		"FifoQueue":                 "true",
		"ContentBasedDeduplication": "false",
	}
	for key, value := range extra {
		attributes[key] = value
	}
	output, err := api.client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: attributes,
	})
	if err != nil {
		return "", fmt.Errorf("create queue %s: %w", name, err)
	}
	return aws.ToString(output.QueueUrl), nil
}

func (api *API) queueARN(ctx context.Context, queueURL string) (string, error) {
	output, err := api.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return "", fmt.Errorf("read queue arn: %w", err)
	}
	return output.Attributes[string(types.QueueAttributeNameQueueArn)], nil
}

func (queue *Queue) Receive(ctx context.Context) (application.QueuedMessage, bool, error) {
	output, err := queue.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queue.url),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     waitTimeSeconds,
		VisibilityTimeout:   visibilityTimeout,
	})
	if err != nil {
		return application.QueuedMessage{}, false, fmt.Errorf("receive wager message: %w", err)
	}
	if len(output.Messages) == 0 {
		return application.QueuedMessage{}, false, nil
	}
	message := output.Messages[0]
	return application.QueuedMessage{
		ID:      aws.ToString(message.MessageId),
		Body:    []byte(aws.ToString(message.Body)),
		Receipt: aws.ToString(message.ReceiptHandle),
	}, true, nil
}

func (queue *Queue) Ready(ctx context.Context) error {
	_, err := queue.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queue.url),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return fmt.Errorf("sqs ready: %w", err)
	}
	return nil
}

func (queue *Queue) release(ctx context.Context, receipt string) error {
	_, err := queue.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(queue.url),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: 0,
	})
	if err != nil {
		return fmt.Errorf("release wager message: %w", err)
	}
	return nil
}

func (queue *Queue) Delete(ctx context.Context, receipt string) error {
	_, err := queue.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queue.url),
		ReceiptHandle: aws.String(receipt),
	})
	if err != nil {
		return fmt.Errorf("delete wager message: %w", err)
	}
	return nil
}

func (queue *Queue) Publish(ctx context.Context, message application.OutboxMessage) error {
	return queue.Send(ctx, string(message.Payload), message.AggregateID, message.ID)
}

func (queue *Queue) Send(ctx context.Context, body string, groupID string, deduplicationID string) error {
	_, err := queue.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(queue.url),
		MessageBody:            aws.String(body),
		MessageGroupId:         aws.String(groupID),
		MessageDeduplicationId: aws.String(deduplicationID),
	})
	if err != nil {
		return fmt.Errorf("send queue message: %w", err)
	}
	return nil
}
