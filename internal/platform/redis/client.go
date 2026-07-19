package redis

import (
	"context"
	"fmt"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

func New(ctx context.Context, cfg config.Redis) (*goredis.Client, error) {
	options, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	options.DialTimeout = cfg.DialTimeout
	options.ReadTimeout = cfg.ReadTimeout
	options.WriteTimeout = cfg.WriteTimeout
	client := goredis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
