package server

import (
	"context"
	"time"

	"github.com/codingsandmore/pumpfun-portal/portal"
	"github.com/codingsandmore/pumpfun-portal/portal/decoders"
	"github.com/codingsandmore/pumpfun-portal/portal/trades"
	"github.com/rs/zerolog/log"
)

type NewPairDiscovered func(p *portal.NewPairResponse)
type NewTradeDiscovered func(p *portal.NewTradeResponse)

type Server interface {
	Discover(pairDiscovery NewPairDiscovered, tradeDiscovery NewTradeDiscovered)
}

type PortalServer struct {
	client  portal.WebSocketClient
	tracker *trades.PriceTracker
	pairs   chan any
	trades  chan *portal.NewTradeResponse
}

// NewPortalServer creates a new PortalServer instance and initializes the WebSocket client
func NewPortalServer() *PortalServer {
	return &PortalServer{
		client:  portal.NewPairClient(),
		tracker: trades.NewPriceTracker(),
		pairs:   make(chan any),
		trades:  make(chan *portal.NewTradeResponse),
	}
}

// Shutdown closes all connections and channels
func (s *PortalServer) Shutdown(ctx context.Context) error {
	log.Info().Msg("shutting down portal server...")

	done := make(chan struct{})
	var shutdownErr error

	// First stop any ongoing subscriptions
	go func() {
		defer close(done)

		// First stop any ongoing subscriptions
		if s.tracker != nil && s.tracker.Client != nil {
			log.Debug().Msg("shutting down tracker client...")
			s.tracker.Client = nil
		}

		// Then close the WebSocket client if it exists
		if s.client != nil {
			log.Debug().Msg("shutting down WebSocket client...")
			if err := s.client.Shutdown(context.Background()); err != nil {
				shutdownErr = err
			}
			s.client = nil
		}

		// Finally close channels
		if s.pairs != nil {
			log.Debug().Msg("closing pairs channel...")
			close(s.pairs)
			s.pairs = nil
		}

		if s.trades != nil {
			log.Debug().Msg("closing trades channel...")
			close(s.trades)
			s.trades = nil
		}
	}()

	select {
	case <-ctx.Done():
		log.Warn().Msg("shutdown timed out, forcing exit")
		return ctx.Err()
	case <-done:
		log.Info().Msg("portal server shutdown complete")
		return shutdownErr
	}
}

// TradeHandler is a function type that handles trade notifications
type TradeHandler func(trade *portal.NewTradeResponse)

// SubscribeToTokenTrades subscribes to trades for specific token addresses and calls the provided handler for each trade
func (s *PortalServer) SubscribeToTokenTrades(tokenAddresses []string, handler TradeHandler) error {
	if len(tokenAddresses) == 0 {
		return nil
	}

	log.Info().Strs("tokenAddresses", tokenAddresses).Msg("subscribing to token trades")

	// Initialize the client if not already done
	if s.client == nil {
		s.client = portal.NewPairClient()
	}

	// Initialize the trades channel if not already done
	if s.trades == nil {
		s.trades = make(chan *portal.NewTradeResponse, 100)
	}

	// Make sure we're connected before subscribing or sending messages
	if err := s.client.Connect(); err != nil {
		log.Error().Err(err).Msg("failed to connect client")
		return err
	}

	time.Sleep(500 * time.Millisecond)

	// Initialize the tracker's client
	if s.tracker.Client == nil {
		s.tracker.Client = s.client
	}

	// Set up trade subscription
	if s.tracker.Client != nil && s.trades != nil {
		// Start the trade tracker
		go func() {
			// channel for trades in the client
			tradesChan := make(chan any, 100)

			// Subscribe to trades with a decoder
			err := s.client.Subscribe(tradesChan, &decoders.TradeDecoder{}, nil)
			if err != nil {
				log.Error().Err(err).Msg("failed to subscribe to trades")
				return
			}
			log.Info().Msg("successfully subscribed to trade decoder")

			// Forward decoded trades to the tracker
			for m := range tradesChan {
				if trade, ok := m.(*portal.NewTradeResponse); ok && trade != nil {
					s.trades <- trade
				}
			}
		}()

		// Start the trade tracker
		go s.tracker.SubscribeToTrades(s.trades)

		// Start the trade handler
		go func() {
			for trade := range s.trades {
				if trade != nil && trade.Signature != "" {
					log.Debug().Str("token", trade.Mint).Msg("received trade")
					go handler(trade)
				}
			}
		}()
	}

	// For each token, create a minimal pair and track it
	for _, tokenAddr := range tokenAddresses {

		pair := &portal.NewPairResponse{
			Mint:      tokenAddr,
			Signature: "manual_subscription",
		}

		// Track the pair for trades
		if err := s.tracker.TrackPair(pair); err != nil {
			log.Error().Err(err).Str("token", tokenAddr).Msg("failed to track token pair")
			continue
		}
		log.Debug().Str("token", tokenAddr).Msg("tracking token pair")
	}

	// Subscribe to the specified tokens
	s.tracker.Client.Send(trades.TrackRequest{
		Method: "subscribeTokenTrade",
		Keys:   tokenAddresses,
	})

	return nil
}

// Discover subscribes to new pairs and their trades, calling the provided callbacks when they're discovered.
// It runs until the context is canceled or an error occurs.
func (s *PortalServer) Discover(pairDiscovery NewPairDiscovered, tradeDiscovery NewTradeDiscovered) {
	// Ensure we're connected
	if err := s.client.Connect(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to WebSocket")
	}

	// Subscribe to new pairs
	go func() {
		err := s.client.Subscribe(s.pairs, &decoders.PairDecoder{}, `{"method": "subscribeNewToken"}`)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to subscribe to pairs")
		}
	}()

	// Start trade tracker
	go s.tracker.SubscribeToTrades(s.trades)

	// Handle incoming pairs
	go func() {
		for m := range s.pairs {
			p, ok := m.(*portal.NewPairResponse)
			if !ok || p == nil || p.Signature == "" {
				log.Debug().Msg("received invalid pair")
				continue
			}

			// Track the pair for trades
			if err := s.tracker.TrackPair(p); err != nil {
				log.Error().Err(err).Msg("failed to track pair")
			}

			// Notify about the new pair
			go pairDiscovery(p)
		}
	}()

	// Forward trades to the discovery callback
	for trade := range s.trades {
		go tradeDiscovery(trade)
	}
}
