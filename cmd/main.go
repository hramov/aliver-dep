package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"leader-election/internal"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"strconv"
	"time"
)

func main() {
	setLogConfigurations()

	if os.Getenv("ALIVER_ENV") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Fatal().Err(fmt.Errorf("error loading .env file"))
		}
	}

	configPath := os.Getenv("ALIVER_CONFIG_PATH")
	if configPath == "" {
		log.Fatal().Err(fmt.Errorf("config path env is not set"))
	}

	cfg := internal.Config{}
	err := internal.LoadConfig(configPath, &cfg)
	if err != nil {
		log.Fatal().Err(fmt.Errorf("cannot parse config file: %v\n", err))
	}

	nodeID := cfg.InstanceID
	if err != nil {
		log.Fatal().Err(err)
	}

	node := internal.NewNode(nodeID, cfg.Servers, cfg.CheckScript, cfg.CheckInterval, cfg.CheckRetries, cfg.CheckTimeout, cfg.RunScript, cfg.RunTimeout, cfg.StopScript, cfg.StopTimeout)

	listener, err := node.NewListener()
	if err != nil {
		log.Fatal().Err(err)
	}
	defer func(listener net.Listener) {
		err = listener.Close()
		if err != nil {
			log.Fatal().Err(err)
		}
	}(listener)

	rpcServer := rpc.NewServer()
	err = rpcServer.Register(node)
	if err != nil {
		log.Fatal().Err(err)
	}

	go rpcServer.Accept(listener)

	node.ConnectToPeers()

	log.Info().Msgf("%s is aware of own peers %s", node.ID, node.Peers.ToIDs())

	warmupTime := 5 * time.Second
	time.Sleep(warmupTime)
	node.Elect()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	node.StopService(context.Background())
}

func setLogConfigurations() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// https://github.com/rs/zerolog#add-file-and-line-number-to-log
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
		return file + ":" + strconv.Itoa(line)
	}

	log.Logger = log.With().Caller().Logger()
}
