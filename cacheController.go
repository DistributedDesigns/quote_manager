package main

import (
	"math/rand"
	"time"

	types "github.com/distributeddesigns/shared_types"

	"github.com/garyburd/redigo/redis"
)

func getCachedQuote(stock string) (types.Quote, bool) {
	conn := redisPool.Get()
	defer conn.Close()

	quoteKey := makeQuoteKey(stock)
	r, err := redis.String(conn.Do("GET", quoteKey))
	if err == redis.ErrNil {
		consoleLog.Debugf(" [x] Cache miss: %s", quoteKey)
		return types.Quote{}, false
	} else if err != nil {
		failOnError(err, "Could not retrieve quote from redis")
	}

	quote, err := types.ParseQuote(r)
	failOnError(err, "Could not parse quote from redis value")

	return quote, true
}

func updateCachedQuote(q types.Quote) {
	quoteAge := time.Now().Unix() - q.Timestamp.Unix()
	ttl := config.QuotePolicy.BaseTTL - rand.Intn(config.QuotePolicy.BackoffTTL) - int(quoteAge)
	quoteKey := makeQuoteKey(q.Stock)
	serializedQuote := q.ToCSV()

	conn := redisPool.Get()
	defer conn.Close()

	_, err := redis.String(conn.Do("SETEX", quoteKey, ttl, serializedQuote))
	failOnError(err, "Could not update quote in redis")

	consoleLog.Debugf("Updated: %s:%s", quoteKey, serializedQuote)
	consoleLog.Debugf("%s will expire in %d sec", quoteKey, ttl)
}

func makeQuoteKey(stock string) string {
	// Truncate the stock to three characters to match the behavior
	// of the quoteserver.
	// TODO: Quote{} should enforce the size limit?

	shortStock := stock[:3]
	return config.Redis.KeyPrefix + "quotes:" + shortStock
}
