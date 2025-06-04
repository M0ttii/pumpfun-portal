package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("failed to shutdown server")
		}
	}()

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
	<-ctx.Done()
	log.Info().Msg("shutting down...")
}
