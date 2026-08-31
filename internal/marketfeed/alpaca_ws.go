package marketfeed

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"quantram/internal/config"
	"quantram/internal/domain"

	"github.com/gorilla/websocket"
)

type AlpacaStream struct {
	url         string
	feed        string
	credentials Credentials
	dialer      *websocket.Dialer

	mu     sync.RWMutex
	health domain.FeedHealth
}

func NewAlpacaStream(url, feed string, credentials Credentials) *AlpacaStream {
	return &AlpacaStream{
		url:         url,
		feed:        feed,
		credentials: credentials,
		dialer:      &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
		health: domain.FeedHealth{
			SourceID: SourceID(feed),
			State:    domain.FeedFailed,
		},
	}
}

func (s *AlpacaStream) Health() domain.FeedHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	health := s.health
	health.SubscribedSymbols = append([]string(nil), s.health.SubscribedSymbols...)
	return health
}

func (s *AlpacaStream) Run(ctx context.Context, symbols []string, out chan<- domain.Bar) error {
	if len(symbols) > config.MaxSymbolsBasicPlan {
		return fmt.Errorf("symbol cap exceeded: %d > %d", len(symbols), config.MaxSymbolsBasicPlan)
	}
	backoff := config.ReconnectBase
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.setState(domain.FeedRecovering, "")
		err := s.session(ctx, symbols, out)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = err
		s.setState(domain.FeedFailed, errorString(err))
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < config.ReconnectCap {
			backoff *= 2
			if backoff > config.ReconnectCap {
				backoff = config.ReconnectCap
			}
		}
		_ = lastErr
	}
}

func (s *AlpacaStream) session(ctx context.Context, symbols []string, out chan<- domain.Bar) error {
	conn, _, err := s.dialer.DialContext(ctx, s.url, http.Header{})
	if err != nil {
		return fmt.Errorf("dial alpaca stream: %w", err)
	}
	defer conn.Close()

	if err := s.authenticate(conn); err != nil {
		return err
	}
	if err := s.subscribe(conn, symbols); err != nil {
		return err
	}
	s.setSubscribed(symbols)
	s.setState(domain.FeedHealthy, "")

	misses := 0
	lastPong := time.Now()
	conn.SetPongHandler(func(string) error {
		now := time.Now()
		rtt := now.Sub(lastPong)
		if rtt < 0 {
			rtt = 0
		}
		s.armReadDeadline(conn)
		s.recordPong(rtt)
		if rtt > config.HeartbeatMaxRTT {
			return fmt.Errorf("pong rtt %s exceeds %s", rtt, config.HeartbeatMaxRTT)
		}
		return nil
	})

	ping := time.NewTicker(config.HeartbeatInterval)
	defer ping.Stop()

	readErr := make(chan error, 1)
	go func() {
		readErr <- s.readLoop(ctx, conn, out)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case t := <-ping.C:
			lastPong = t
			deadline := t.Add(config.HeartbeatInterval)
			if err := conn.WriteControl(websocket.PingMessage, []byte("quantram"), deadline); err != nil {
				misses++
				s.recordMiss(uint32(misses))
				if misses >= config.HeartbeatMaxMisses {
					return fmt.Errorf("heartbeat write failed %d times: %w", misses, err)
				}
				continue
			}
			misses = 0
			s.recordMiss(0)
		}
	}
}

func (s *AlpacaStream) authenticate(conn *websocket.Conn) error {
	greeting, err := readControl(conn)
	if err != nil {
		return fmt.Errorf("read alpaca greeting: %w", err)
	}
	if greeting.Type == "error" {
		return controlError("greeting", greeting)
	}
	if greeting.Message != "authenticated" {
		auth := map[string]string{
			"action": "auth",
			"key":    s.credentials.Key,
			"secret": s.credentials.Secret,
		}
		if err := conn.WriteJSON(auth); err != nil {
			return fmt.Errorf("write auth: %w", err)
		}
		response, err := readControl(conn)
		if err != nil {
			return fmt.Errorf("read auth response: %w", err)
		}
		if response.Type == "error" {
			return controlError("auth", response)
		}
		if response.Message != "authenticated" {
			return fmt.Errorf("alpaca auth incomplete: type=%s msg=%s", response.Type, response.Message)
		}
	}
	s.recordMessage()
	return nil
}

func (s *AlpacaStream) subscribe(conn *websocket.Conn, symbols []string) error {
	msg := map[string]any{
		"action":      "subscribe",
		"bars":        symbols,
		"updatedBars": symbols,
	}
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write subscribe: %w", err)
	}
	control, err := readControl(conn)
	if err != nil {
		return fmt.Errorf("read subscribe: %w", err)
	}
	if control.Type == "error" {
		return controlError("subscribe", control)
	}
	log.Printf("alpaca subscribed type=%s bars=%v", control.Type, control.Bars)
	s.recordMessage()
	return nil
}

func readControl(conn *websocket.Conn) (alpacaControl, error) {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return alpacaControl{}, err
	}
	messages, err := decodeMessageArray(raw)
	if err != nil {
		return alpacaControl{}, err
	}
	if len(messages) == 0 {
		return alpacaControl{}, fmt.Errorf("empty alpaca control")
	}
	return decodeControl(messages[0])
}

func controlError(stage string, control alpacaControl) error {
	return fmt.Errorf("alpaca %s error %d %s", stage, control.Code, control.Message)
}

func (s *AlpacaStream) readLoop(ctx context.Context, conn *websocket.Conn, out chan<- domain.Bar) error {
	source := SourceID(s.feed)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.armReadDeadline(conn)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read alpaca stream: %w", err)
		}
		s.recordMessage()
		messages, err := decodeMessageArray(raw)
		if err != nil {
			continue
		}
		receipt := time.Now().UTC()
		for _, message := range messages {
			control, _ := decodeControl(message)
			if control.Type == "error" {
				return fmt.Errorf("alpaca stream error %d %s", control.Code, control.Message)
			}
			if control.Type != "b" && control.Type != "u" {
				continue
			}
			bar, err := barFromRaw(message, source, receipt, false)
			if err != nil {
				log.Printf("skip alpaca bar: %v", err)
				continue
			}
			select {
			case out <- bar:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (s *AlpacaStream) armReadDeadline(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(config.StreamReadIdle))
}

func (s *AlpacaStream) setState(state domain.FeedState, lastError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.State = state
	s.health.LastError = lastError
}

func (s *AlpacaStream) setSubscribed(symbols []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.SubscribedSymbols = append([]string(nil), symbols...)
}

func (s *AlpacaStream) recordMessage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.LastMessage = time.Now()
	s.health.ConsecutiveHeartbeatFailures = 0
}

func (s *AlpacaStream) recordPong(rtt time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.LastPongRTT = rtt
	s.health.LastMessage = time.Now()
	s.health.ConsecutiveHeartbeatFailures = 0
}

func (s *AlpacaStream) recordMiss(count uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.ConsecutiveHeartbeatFailures = count
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
