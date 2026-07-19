package rabbitmq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitConn interface {
	Channel() (rabbitChannel, error)
	Close() error
}

type rabbitChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Close() error
}

type amqpConnAdapter struct{ conn *amqp.Connection }

type amqpChannelAdapter struct{ ch *amqp.Channel }

func (c amqpConnAdapter) Channel() (rabbitChannel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	return amqpChannelAdapter{ch: ch}, nil
}
func (c amqpConnAdapter) Close() error { return c.conn.Close() }

func (c amqpChannelAdapter) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	return c.ch.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

func (c amqpChannelAdapter) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return c.ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

func (c amqpChannelAdapter) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	return c.ch.QueueBind(name, key, exchange, noWait, args)
}

func (c amqpChannelAdapter) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	return c.ch.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}

func (c amqpChannelAdapter) Qos(prefetchCount, prefetchSize int, global bool) error {
	return c.ch.Qos(prefetchCount, prefetchSize, global)
}

func (c amqpChannelAdapter) ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return c.ch.ConsumeWithContext(ctx, queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}
func (c amqpChannelAdapter) Close() error { return c.ch.Close() }

func defaultDialRabbit(cfg Config) (rabbitConn, error) {
	conn, err := dial(cfg)
	if err != nil {
		return nil, err
	}
	return amqpConnAdapter{conn: conn}, nil
}
