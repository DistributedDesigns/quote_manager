package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
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
	logLevels = []string{"CRITICAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG"}
	logLevel  = kingpin.
			Flag("log-level", fmt.Sprintf("Minimum level for logging to the console. Must be one of: %s", strings.Join(logLevels, ", "))).
			Default("WARNING").
			Short('l').
			Enum(logLevels...)
	serviceID = kingpin.
			Flag("service-id", "Logging name for the service").
			Default("quotemgr").
			Short('s').
			String()
	configFile = kingpin.
			Flag("config", "YAML file with service config").
			Default("./config/dev.yaml").
			Short('c').
			ExistingFile()

	consoleLog       = logging.MustGetLogger("console")
	pendingQuoteReqs = make(chan amqp.Delivery)

	rmqConn   *amqp.Connection
	redisPool *redis.Pool
)

func main() {
	rand.Seed(time.Now().Unix())

	kingpin.Parse()
	initConsoleLogging()
	loadConfig()
	initRMQ()
	defer rmqConn.Close()
	initRedis()

	forever := make(chan bool)

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
		`%{time:15:04:05.000} %{color}▶ %{level:8s}%{color:reset} %{id:03d} %{message}  %{shortfile}`,
	)
	consoleBackendFormatted := logging.NewBackendFormatter(consoleBackend, consoleFormat)

	// Add leveled logging
	level, err := logging.LogLevel(*logLevel)
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
	Rabbit struct {
		Host   string
		Port   int
		User   string
		Pass   string
		Queues struct {
			QuoteRequest string `yaml:"quote request"`
		}
		Exchanges struct {
			QuoteBroadcast string `yaml:"quote broadcast"`
		}
	}

	QuoteServer struct {
		Host string
		Port int
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
	// Load the yaml file
	data, err := ioutil.ReadFile(*configFile)
	failOnError(err, "Could not read file")

	err = yaml.Unmarshal(data, &config)
	failOnError(err, "Could not unmarshal config")
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
		config.Rabbit.Queues.QuoteRequest, // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// Broadcasting quote updates
	err = ch.ExchangeDeclare(
		config.Rabbit.Exchanges.QuoteBroadcast, // name
		amqp.ExchangeTopic,                     // type
		true,                                   // durable
		false,                                  // auto-deleted
		false,                                  // internal
		false,                                  // no-wait
		nil,                                    // args
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
