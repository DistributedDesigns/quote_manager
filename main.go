package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/op/go-logging"
	"github.com/streadway/amqp"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	yaml "gopkg.in/yaml.v2"
)

// Globals
var (
	consoleLog       = logging.MustGetLogger("console")
	pendingQuoteReqs = make(chan amqp.Delivery)

	rmqConn   *amqp.Connection
	redisPool *redis.Pool

	quoteServerTCPAddr *net.TCPAddr
)

const (
	// Shared RMQ queues / exchanges
	quoteRequestQ    = "quote_req"
	quoteBroadcastEx = "quote_broadcast"
)

func main() {
	rand.Seed(time.Now().Unix())

	loadConfig()
	initConsoleLogging()
	resolveTCPAddresses()
	initRMQ()
	defer rmqConn.Close()
	initRedis()

	forever := make(chan bool)

	go quoteCatcher()
	go handleQuoteRequest()
	go handleQuoteBroadcast()

	<-forever
}

func failOnError(err error, msg string) {
	if err != nil {
		consoleLog.Fatalf("%s: %s", msg, err)
	}
}

func initConsoleLogging() {

	// Create a default backend
	consoleBackend := logging.NewLogBackend(os.Stdout, "", 0)

	// Add output formatting
	var consoleFormat = logging.MustStringFormatter(
		`%{time:15:04:05.000} %{color}▶ %{level:8s}%{color:reset} %{id:03d} %{message} (%{shortfile})`,
	)
	consoleBackendFormatted := logging.NewBackendFormatter(consoleBackend, consoleFormat)

	// Add leveled logging
	level, err := logging.LogLevel(config.env.logLevel)
	if err != nil {
		fmt.Println("Bad log level. Using default level of ERROR")
	}
	consoleBackendFormattedAndLeveled := logging.AddModuleLevel(consoleBackendFormatted)
	consoleBackendFormattedAndLeveled.SetLevel(level, "")

	// Attach the backend
	logging.SetBackend(consoleBackendFormattedAndLeveled)
}

// Holds values from <config>.yaml.
// 'PascalCase' values come from 'pascalcase' in x.yaml
var config struct {
	// Stuff from the command line, through kingpin
	env struct {
		logLevel   string
		serviceID  string
		configFile string
	}

	// Stuff from ./config/$env.yaml
	Rabbit struct {
		Host string
		Port int
		User string
		Pass string
	}

	QuoteServer struct {
		Host    string
		Port    int
		Retry   int
		Backoff int
	} `yaml:"quote server"`

	Redis struct {
		Host        string
		Port        int
		MaxIdle     int    `yaml:"max idle connections"`
		MaxActive   int    `yaml:"max active connections"`
		IdleTimeout int    `yaml:"idle timeout"`
		KeyPrefix   string `yaml:"key prefix"`
	}

	QuotePolicy struct {
		BaseTTL    int `yaml:"base ttl"`
		BackoffTTL int `yaml:"backoff ttl"`
	} `yaml:"quote policy"`
}

func loadConfig() {
	app := kingpin.New("quote_manager", "Get quotes from the legacy service")

	var logLevels = []string{"CRITICAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG"}

	app.Flag("log-level", fmt.Sprintf("Minimum level for logging to the console. Must be one of: %s", strings.Join(logLevels, ", "))).
		Default("WARNING").
		Short('l').
		EnumVar(&config.env.logLevel, logLevels...)

	app.Flag("service-id", "Logging name for the service").
		Default("quotemgr").
		Short('s').
		StringVar(&config.env.serviceID)

	app.Flag("config", "YAML file with service config").
		Default("./config/dev.yaml").
		Short('c').
		ExistingFileVar(&config.env.configFile)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Load the yaml file
	data, err := ioutil.ReadFile(config.env.configFile)
	if err != nil {
		// `consoleLog` not set up yet so fail the old-fashioned way.
		panic(err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		panic(err)
	}
}

func resolveTCPAddresses() {
	// TCP connections need to know the specific IP for the destination.
	// We can do the lookup in advance since destinations are fixed.
	quoteServerAddress := fmt.Sprintf("%s:%d",
		config.QuoteServer.Host, config.QuoteServer.Port,
	)

	var err error
	quoteServerTCPAddr, err = net.ResolveTCPAddr("tcp", quoteServerAddress)
	failOnError(err, "Could not resolve TCP addr for "+quoteServerAddress)
}

func initRMQ() {
	rabbitAddress := fmt.Sprintf("amqp://%s:%s@%s:%d",
		config.Rabbit.User, config.Rabbit.Pass,
		config.Rabbit.Host, config.Rabbit.Port,
	)

	var err error
	rmqConn, err = amqp.Dial(rabbitAddress)
	failOnError(err, "Failed to rmqConnect to RabbitMQ")
	// closed in main()

	ch, err := rmqConn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	// Make sure all of the expected RabbitMQ exchanges and queues
	// exist before we start using them.
	// Recieve requests
	_, err = ch.QueueDeclare(
		quoteRequestQ, // name
		true,          // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no wait
		nil,           // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// Broadcasting quote updates
	err = ch.ExchangeDeclare(
		quoteBroadcastEx,   // name
		amqp.ExchangeTopic, // type
		true,               // durable
		false,              // auto-deleted
		false,              // internal
		false,              // no-wait
		nil,                // args
	)
	failOnError(err, "Failed to declare an exchange")
}

func initRedis() {
	redisAddress := fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port)

	redisPool = &redis.Pool{
		MaxIdle:     config.Redis.MaxIdle,
		MaxActive:   config.Redis.MaxActive,
		IdleTimeout: time.Second * time.Duration(config.Redis.IdleTimeout),
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", redisAddress) },
	}

	// Test if we can talk to redis
	conn := redisPool.Get()
	defer conn.Close()

	_, err := conn.Do("PING")
	failOnError(err, "Could not establish connection with Redis")
}
