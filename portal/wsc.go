package portal

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type MessageDecoder interface {
	Decode(bytes []byte) (any, error)
}
type WebSocketClient interface {
	Connect() error
	Send(any)
	Subscribe(messasge chan any, decoder MessageDecoder, welcomeMessage any) error
	Shutdown(context.Context) error
}
type DefaultWebSocketClient struct {
	Addr             string
	Conn             *websocket.Conn
	IncomingMessages chan any
	OutgoingMessages chan any
	Decoder          MessageDecoder
	welcomeMessage   any
}

func (c *DefaultWebSocketClient) Shutdown(ctx context.Context) error {
	log.Debug().Msg("shutting down WebSocket client...")

	if c.Conn != nil {

		if err := c.Conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(5*time.Second),
		); err != nil && !websocket.IsUnexpectedCloseError(err) {
			return err
		}

		if err := c.Conn.Close(); err != nil {
			log.Warn().Err(err).Msg("error closing WebSocket connection")
		}
		c.Conn = nil
	}

	if c.OutgoingMessages != nil {
		log.Debug().Msg("closing outgoing messages channel")
		close(c.OutgoingMessages)
		c.OutgoingMessages = nil
	}

	if c.IncomingMessages != nil {
		log.Debug().Msg("closing incoming messages channel")
		close(c.IncomingMessages)
		c.IncomingMessages = nil
	}

	log.Debug().Msg("WebSocket client shutdown complete")
	return nil
}
func NewDefaultClient(addr string) *DefaultWebSocketClient {

	return &DefaultWebSocketClient{
		Addr:             addr,
		Conn:             nil,
		IncomingMessages: make(chan any),
		OutgoingMessages: make(chan any, 10), // Change 10 to appropriate buffer size
	}
}

func (c *DefaultWebSocketClient) Subscribe(messages chan any, decoder MessageDecoder, welcomeMessage any) error {
	if decoder == nil {
		log.Error().Msgf("you need to provide a decoder or this process will fail!")
		return errors.New("please provide a decoder")
	} else {
		log.Info().Any("decoder", decoder).Msgf("subscribing to messages")
	}
	c.welcomeMessage = welcomeMessage
	c.Decoder = decoder
	c.Start()

	for {
		select {
		case m := <-c.IncomingMessages:
			if m != nil {
				messages <- m
			} else {
				log.Warn().Msgf("ignoring Nil message!")
			}

		}
	}
}
func (c *DefaultWebSocketClient) Connect() error {
	log.Debug().Str("url", c.Addr).Msgf("connecting to %v", c.Addr)
	u, err := url.Parse(c.Addr)
	if err != nil {
		log.Error().AnErr("error during parsing of url", err)
		return err
	}

	c.Conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Error().AnErr("error during dialing url", err)
		return err
	}

	if c.welcomeMessage != nil {
		log.Debug().Any("welcome", c.welcomeMessage).Msgf("sending welcome message")
		c.Send(c.welcomeMessage)
	}
	return nil
}

func (c *DefaultWebSocketClient) maintainConnection() {
	for {
		if c.Conn == nil {
			log.Info().Msgf("attempting to establish connection to %v", c.Addr)
			if err := c.Connect(); err != nil {
				log.Error().AnErr("connect error", err).Msgf("Connect Error:", err)
				time.Sleep(time.Second)
				continue
			}
		}

		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			log.Error().AnErr("read error", err).Msgf("Read Error:", err)
			c.Conn = nil
			continue
		}

		message, err := c.Decoder.Decode(msg)

		if err != nil {
			log.Error().AnErr("decode error", err).Msgf("Decode Error: %s", err)
		} else if message == nil {
			log.Warn().Msgf("ignored nil message")
		} else {
			log.Debug().Any("message", message).Msgf("forwarding decoded message")
			c.IncomingMessages <- message
		}
	}
}

func (c *DefaultWebSocketClient) maintainOutgoingMessages() {
	for msg := range c.OutgoingMessages {
		if c.Conn == nil {
			log.Error().Msgf("Connection Error: No active connection, Sending message back to queue")
			time.Sleep(500 * time.Millisecond)
			c.OutgoingMessages <- msg
			continue
		}

		var err error
		switch v := msg.(type) {
		case []byte:
			err = c.Conn.WriteMessage(1, v)
		case string:
			err = c.Conn.WriteMessage(1, []byte(v))

		default:
			err = c.Conn.WriteJSON(v)
		}

		if err != nil {
			log.Error().AnErr("write error", err).Msgf("Write Error:", err)
			c.Conn = nil
			continue
		}
	}
}

func (c *DefaultWebSocketClient) Send(msg any) {
	log.Debug().Any("message", msg).Msg("Sending message")
	c.OutgoingMessages <- msg
}

func (c *DefaultWebSocketClient) Start() {
	go c.maintainConnection()
	go c.maintainOutgoingMessages()
}
