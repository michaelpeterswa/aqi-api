package dragonfly

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

var (
	ErrUnableToPingDragonfly = errors.New("unable to ping dragonfly")
)

type DragonflyClient struct {
	Client *redis.Client
	//TODO: consider turning into map
	CacheResultsDuration time.Duration
	KeyPrefix            string
}

func NewDragonflyClient(host string, port int, password string, cacheResultsDuration time.Duration, keyPrefix string) (*DragonflyClient, error) {
	redisOpts := &redis.Options{
		Addr: fmt.Sprintf("%s:%d", host, port),
		DB:   0, // use default DB
	}

	if password != "" {
		redisOpts.Password = password
	}

	redisClient := redis.NewClient(redisOpts)

	// Span cache operations so they nest under the request span, next to the
	// otelpgx database spans. A no-op while tracing is disabled.
	if err := redisotel.InstrumentTracing(redisClient); err != nil {
		return nil, fmt.Errorf("instrument dragonfly tracing: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second*10)
	defer pingCancel()

	_, err := redisClient.Ping(pingCtx).Result()
	if err != nil {
		return nil, err
	}

	return &DragonflyClient{
		Client:               redisClient,
		CacheResultsDuration: cacheResultsDuration,
		KeyPrefix:            keyPrefix,
	}, nil
}

func (dc *DragonflyClient) GetClient() *redis.Client {
	return dc.Client
}
