package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultQueueName = "jobs"

type Queue struct {
	client *redis.Client
	name   string
}

func New(addr string) (*Queue, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Queue{client: client, name: defaultQueueName}, nil
}

func (q *Queue) Close() error {
	return q.client.Close()
}

func (q *Queue) Enqueue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, q.name, jobID).Err()
}

func (q *Queue) Dequeue(ctx context.Context, timeout time.Duration) (string, error) {
	result, err := q.client.BRPop(ctx, timeout, q.name).Result()
	if err != nil {
		return "", err
	}
	if len(result) < 2 {
		return "", redis.Nil
	}
	return result[1], nil
}
