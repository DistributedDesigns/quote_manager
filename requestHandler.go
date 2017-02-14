package main

func handleQuoteRequest() {

	ch, err := rmqConn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	msgs, err := ch.Consume(
		quoteRequestQ, // queue
		"",            // consumer
		false,         // auto-ack
		false,         // exclusive
		false,         // no-local
		false,         // no-wait
		nil,           // args
	)
	failOnError(err, "Failed to register a consumer")

	forever := make(chan bool)

	go func() {
		consoleLog.Infof(" [-] Monitoring %s", quoteRequestQ)

		for d := range msgs {
			consoleLog.Infof(" [↓] Request: TxID %s, '%s'", d.CorrelationId, d.Body)
			consoleLog.Debugf("Headers: %+v", d.Headers)
			pendingQuoteReqs <- d
			d.Ack(false)
		}
	}()

	<-forever
}
