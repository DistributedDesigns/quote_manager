package main

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/distributeddesigns/shared_types"

	"github.com/streadway/amqp"
)

func handleQuoteBroadcast() {
	forever := make(chan bool)

	go func() {
		consoleLog.Info(" [-] Waiting for new pending quotes")

		for req := range pendingQuoteReqs {
			go retrieveAndPublishQuote(req)
		}
	}()

	<-forever
}

func retrieveAndPublishQuote(req amqp.Delivery) {
	qr, err := types.ParseQuoteRequest(string(req.Body))
	failOnError(err, "Could not parse quote request")

	// Quotes are published to XYZ.cached or XYZ.fresh depending on
	// cache hit / miss. Optimistically assume hit.
	routingSuffix := ".cached"
	quote, found := getCachedQuote(qr.Stock)
	if !found || !qr.AllowCache {
		consoleLog.Noticef(" [⟳] Getting new quote for %s", qr.Stock)
		quote = getNewQuote(qr)
		routingSuffix = ".fresh"
		go updateCachedQuote(quote)
	}

	ch, err := rmqConn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	header := amqp.Table{
		"serviceID": *serviceID,
	}

	err = ch.Publish(
		quoteBroadcastEx,       // exchange
		qr.Stock+routingSuffix, // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Headers:       header,
			CorrelationId: req.CorrelationId,
			ContentType:   "text/plain",
			Body:          []byte(quote.ToCSV()),
		})
	failOnError(err, "Failed to publish a message")

	req.Ack(false)

	consoleLog.Infof(" [↑] Broadcast: TxID %d %s %s", quote.ID, quote.Stock, quote.Price.String())
}

func getNewQuote(qr types.QuoteRequest) types.Quote {
	quoteServerAddress := fmt.Sprintf("%s:%d",
		config.QuoteServer.Host, config.QuoteServer.Port,
	)

	// quoteserve.seng reads until it sees a \n
	quoteServerMessage := fmt.Sprintf("%s,%s\n", qr.Stock, qr.UserID)

	readTimeoutBase := time.Millisecond * time.Duration(config.QuoteServer.Retry)
	backoff := time.Millisecond * time.Duration(config.QuoteServer.Backoff)

	respBuf := make([]byte, 1024)
	attempts := 1

	// Loop until read completes or deadline arrives. Do exponential backoff
	// if we time out.
	for {
		consoleLog.Debug("Attempt", attempts)

		// Get a new connection
		quoteServerConn, err := net.DialTimeout("tcp", quoteServerAddress, time.Second*5)
		failOnError(err, "Could not connect to "+quoteServerAddress)

		// Send the message
		quoteServerConn.Write([]byte(quoteServerMessage))

		// Set the deadline
		timeout := readTimeoutBase + backoff
		quoteServerConn.SetReadDeadline(time.Now().Add(timeout))

		// Wait for read or timeout
		_, err = quoteServerConn.Read(respBuf)

		// We either read successfuly or we need to backoff and make
		// a new connection.
		quoteServerConn.Close()

		// If everything was okay then we got a response
		if err == nil {
			// Exit the loop
			break
		}

		// Don't back off forever. Max delay from quote server is 4s.
		// Fail if we waited > 5s so we can investigate.
		if timeout > time.Second*5 {
			consoleLog.Fatalf("No response from %s after %d ms", quoteServerAddress, timeout/1e6)
		}

		// check for a timeout
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// do the backoff and try again
			backoff *= 2
			consoleLog.Debugf("Attempt %d timeout. Waiting for %d ms", attempts, timeout/1e6)
		} else {
			failOnError(err, "Failed to read from quoteserver")
		}

		attempts++
	}

	// clean up the unused space in the buffer
	respBuf = bytes.Trim(respBuf, "\x00")

	// Append the quote request transaction ID to the quote so the ID
	// will be associated with the object when it's parsed.
	respWithTxID := fmt.Sprintf("%s,%d", string(respBuf), qr.ID)
	quote, err := types.ParseQuote(respWithTxID)
	failOnError(err, "Could not parse quote response")

	return quote
}
