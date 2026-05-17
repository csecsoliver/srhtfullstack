package updatehook

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	goredis "github.com/go-redis/redis/v8"
	"github.com/vaughan0/go-ini"
)

var options map[string]string

func loadOptions(logger *log.Logger, config ini.File) {
	if options != nil {
		return
	}

	uuid, ok := os.LookupEnv("SRHT_PUSH")
	if !ok {
		return
	}

	redisHost, ok := config.Get("sr.ht", "redis-host")
	if !ok {
		redisHost = "redis://localhost:6379"
	}
	ropts, err := goredis.ParseURL(redisHost)
	if err != nil {
		logger.Fatalf("Failed to parse redis host: %v", err)
	}
	redis := goredis.NewClient(ropts)

	var n int
	if nopts, ok := os.LookupEnv("GIT_PUSH_OPTION_COUNT"); ok {
		n, _ = strconv.Atoi(nopts)
		redis.Set(context.Background(), fmt.Sprintf("git.sr.ht.options.%s", uuid),
			nopts, 10*time.Minute)
	} else {
		nopts, err := redis.Get(context.Background(), fmt.Sprintf(
			"git.sr.ht.options.%s", uuid)).Result()
		if err != nil {
			return
		}
		n, _ = strconv.Atoi(nopts)
	}

	options = make(map[string]string)
	for i := 0; i < n; i++ {
		opt, ok := os.LookupEnv(fmt.Sprintf("GIT_PUSH_OPTION_%d", i))
		optkey := fmt.Sprintf("git.sr.ht.options.%s.%d", uuid, i)
		if !ok {
			opt, err = redis.Get(context.Background(), optkey).Result()
			if err != nil {
				return
			}
		} else {
			redis.Set(context.Background(), optkey, opt, 10*time.Minute)
		}
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) == 1 {
			options[parts[0]] = ""
		} else {
			options[parts[0]] = parts[1]
		}
	}
}
