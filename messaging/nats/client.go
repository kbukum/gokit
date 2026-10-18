package nats

import (
	"context"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

type natsConn interface {
	PublishMsg(*natsgo.Msg) error
	FlushTimeout(time.Duration) error
	Drain() error
	Close()
	IsClosed() bool
	SubscribeSync(string) (natsSubscription, error)
	QueueSubscribeSync(string, string) (natsSubscription, error)
}

type natsSubscription interface {
	NextMsgWithContext(context.Context) (*natsgo.Msg, error)
	Unsubscribe() error
}

type natsConnAdapter struct{ conn *natsgo.Conn }

func (c natsConnAdapter) PublishMsg(msg *natsgo.Msg) error   { return c.conn.PublishMsg(msg) }
func (c natsConnAdapter) FlushTimeout(d time.Duration) error { return c.conn.FlushTimeout(d) }
func (c natsConnAdapter) Drain() error                       { return c.conn.Drain() }
func (c natsConnAdapter) Close()                             { c.conn.Close() }
func (c natsConnAdapter) IsClosed() bool                     { return c.conn.IsClosed() }
func (c natsConnAdapter) SubscribeSync(subject string) (natsSubscription, error) {
	return c.conn.SubscribeSync(subject)
}

func (c natsConnAdapter) QueueSubscribeSync(subject, queue string) (natsSubscription, error) {
	return c.conn.QueueSubscribeSync(subject, queue)
}

func defaultConnectNATS(url string, opts ...natsgo.Option) (natsConn, error) {
	conn, err := natsgo.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	return natsConnAdapter{conn: conn}, nil
}

// Connect validates cfg and opens a connection with its authentication, TLS, timeout and reconnect settings, followed
// by extra options. Adapters built on NATS, such as JetStream, share it so every connection is configured alike.
func Connect(cfg Config, extra ...natsgo.Option) (*natsgo.Conn, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts, err := cfg.connectOptions()
	if err != nil {
		return nil, err
	}
	conn, err := natsgo.Connect(cfg.URL, append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("nats connect %s: %w", cfg.RedactedURL(), err)
	}
	return conn, nil
}
