package main

import (
	"fmt"

	"github.com/distributeddesigns/shared_types"
)

// quoteCatcher intercepts quotes broadcast by other quote servers.
func quoteCatcher() {
	ch, err := rmqConn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	qName := fmt.Sprintf("quote_mgr:intercept:%s", config.env.serviceID)
	q, err := ch.QueueDeclare(
		qName, // name
		true,  // durable
		true,  // delete when unused
		false, // exclusive
		false, // no wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// Receive all fresh quotes
	freshQuotes := "*.fresh"
	err = ch.QueueBind(
		q.Name,           // name
		freshQuotes,      // routing key
		quoteBroadcastEx, // exchange
		false,            // no-wait
		nil,              // args
	)
	failOnError(err, "Failed to bind a queue")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	consoleLog.Info(" [-] Intercepting quotes not sent from", config.env.serviceID)

	for d := range msgs {
		// Discard messages that we sent
		if d.Headers["serviceID"] == config.env.serviceID {
			continue
		}

		q, err := types.ParseQuote(string(d.Body))
		if err != nil {
			consoleLog.Error(" [x] Bad quote??:", err.Error())
			continue
		}

		consoleLog.Debug(" [↙] Intercepted quote from", d.Headers["serviceID"])

		go updateCachedQuote(q)
	}

	consoleLog.Critical("YOU WILL NEVER GET THIS")
}
