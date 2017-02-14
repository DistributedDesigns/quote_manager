package main

import (
	"bufio"
	"fmt"
	"io"
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
	if !found {
		consoleLog.Noticef(" [⟳] Getting new quote for %s", qr.Stock)
		quote = getNewQuote(qr)
		routingSuffix = ".fresh"
		go updateCachedQuote(quote)
	}

	ch, err := rmqConn.Channel()
	failOnError(err, "Failed to open a channel")

	header := amqp.Table{
		"serviceID":     *serviceID,
		"transactionID": req.Headers["transactionID"],
		"userID":        req.Headers["userID"],
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

	consoleLog.Infof(" [↑] Broadcast: TxID %d %s %s", header["transactionID"], quote.Stock, quote.Price.String())
}

func getNewQuote(qr types.QuoteRequest) types.Quote {
	quoteServerAddress := fmt.Sprintf("%s:%d",
		config.QuoteServer.Host, config.QuoteServer.Port,
	)

	quoteServerrmqConn, err := net.DialTimeout("tcp", quoteServerAddress, time.Second*5)
	failOnError(err, "Could not rmqConnect to "+quoteServerAddress)
	defer quoteServerrmqConn.Close()

	// quoteserve.seng reads until it sees a \n
	quoteServerMessage := qr.ToCSV() + "\n"
	quoteServerrmqConn.Write([]byte(quoteServerMessage))
	// TODO: retry on timeout?

	response, err := bufio.NewReader(quoteServerrmqConn).ReadString('\n')
	// when stream is done an EOF is emitted that we should ignore
	if err != io.EOF && err != nil {
		// TODO: retry on timeout?
		failOnError(err, "Failed to read quote server response")
	}

	quote, err := types.ParseQuote(response)
	failOnError(err, "Could not parse quote response")

	return quote
}
