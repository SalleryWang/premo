package bitxhub

import (
	"sync"
	"sync/atomic"
	"time"

	rpcx "github.com/meshplus/go-bitxhub-client"
	"github.com/sirupsen/logrus"
	"github.com/wonderivan/logger"
)

const (
	DefaultTo = "000000000000000000000000000000000000000a"
)

var log = logrus.New()
var cfg = &config{
	addrs: []string{
		"localhost:60011",
	},
	logger: log,
}

type config struct {
	addrs  []string
	logger rpcx.Logger
}

type Broker struct {
	config *Config
	bees   []*bee
}

type Config struct {
	Concurrent  int
	TPS         int
	Duration    int // s uint
	Type        string
	Validator   string
	Rule        []byte
	KeyPath     string
	BitxhubAddr []string
}

func New(config *Config) (*Broker, error) {
	log.WithFields(logrus.Fields{
		"concurrent": config.Concurrent,
		"tps":        config.TPS,
		"duration":   config.Duration,
		"type":       config.Type,
	}).Info("Premo configuration")

	bees := make([]*bee, 0, config.Concurrent)

	var wg sync.WaitGroup
	wg.Add(config.Concurrent)

	for i := 0; i < config.Concurrent; i++ {
		go func() {
			defer wg.Done()

			bee, err := NewBee(config.TPS/config.Concurrent, config.KeyPath, config.BitxhubAddr)
			if err != nil {
				logger.Error(err)
				return
			}

			if config.Type == "interchain" {
				if err := bee.prepareChain("fabric", "检查链", config.Validator, "1.4.4", "fabric for law", config.Rule); err != nil {
					logger.Error(err)
					return
				}
			}

			bees = append(bees, bee)
		}()
	}

	wg.Wait()
	log.WithFields(logrus.Fields{
		"number": len(bees),
	}).Info("generate all bees")

	return &Broker{
		config: config,
		bees:   bees,
	}, nil
}

func (broker *Broker) Start(typ string) error {
	logger.Info("starting broker")
	var wg sync.WaitGroup
	wg.Add(len(broker.bees))

	current := time.Now()
	lastCounter := atomic.LoadInt64(&counter)
	lastDelayer := atomic.LoadInt64(&delayer)
	for i := 0; i < len(broker.bees); i++ {
		go func(i int) {
			err := broker.bees[i].start(typ)
			if err != nil {
				logger.Error(err)
				return
			}

		}(i)
		log.WithFields(logrus.Fields{
			"index": i + 1,
		}).Debug("start bee")
		wg.Done()
	}
	wg.Wait()
	log.WithFields(logrus.Fields{
		"number": len(broker.bees),
	}).Info("start all bees")

	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			<-ticker.C
			currentCounter := atomic.LoadInt64(&counter)
			c := float64(currentCounter - lastCounter)
			lastCounter = currentCounter

			currentDelayer := atomic.LoadInt64(&delayer)
			d := float64(currentDelayer-lastDelayer) / float64(time.Millisecond)
			lastDelayer = currentDelayer
			log.Infof("current tps is %f, tx_delay is %fms", c, d/c)
		}
	}()

	time.Sleep(time.Duration(broker.config.Duration) * time.Second)

	_ = broker.Stop(current)
	return nil
}

func (broker *Broker) Stop(current time.Time) error {
	for i := 0; i < len(broker.bees); i++ {
		broker.bees[i].stop()
	}
	delayerAvg := float64(delayer) / float64(counter)
	log.WithFields(logrus.Fields{
		"number":   counter,
		"duration": time.Since(current).Seconds(),
		"tps":      float64(counter) / time.Since(current).Seconds(),
		"tx_delay": delayerAvg / float64(time.Millisecond),
	}).Info("finish testing")

	return nil
}
