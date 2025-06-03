package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/codingsandmore/pumpfun-portal/portal"
	"github.com/codingsandmore/pumpfun-portal/portal/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Create server instance
	srv := server.NewPortalServer()
	defer srv.Shutdown()

	// Subscribe to specific token trades
	tokenAddresses := []string{"2uoLkN6jWZsTvzYZkaG86Xjg4XQR8rcHUJp9gCP7pump"}
	tradeHandler := func(trade *portal.NewTradeResponse) {
		log.Info().Any("trade", trade).Msg("received trade for subscribed token")
	}
	if err := srv.SubscribeToTokenTrades(tokenAddresses, tradeHandler); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to token trades")
	}

	// Example 2: Or use Discover to get all new pairs and their trades
	/*
		discoverPair := func(p *portal.NewPairResponse) {
			log.Info().Any("pair", p).Msgf("discovered pair")
		}
		discoverTrade := func(p *portal.NewTradeResponse) {
			log.Info().Any("trade", p).Msg("discovered trade")
		}
		go srv.Discover(discoverPair, discoverTrade)
	*/

	// Wait for interrupt signal to gracefully shut down
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	<-sigs
	log.Info().Msg("shutting down...")
}
