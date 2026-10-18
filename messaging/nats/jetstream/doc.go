// Package jetstream is the durable NATS messaging adapter. It implements the shared messaging ports with at-least-once
// delivery: a [Producer] stores each message in a stream and waits for the server's acknowledgement, and a [Consumer]
// reads a durable pull consumer and acknowledges a message only after its handler succeeds.
//
// Beyond the shared ports it declares streams ([Client.EnsureStream]) and reads retained messages without changing any
// durable consumer ([Client.Replay]). [Component] owns a shared [Client] for a component registry, and [Register] adds
// the adapter to a [messaging.Registry] under [AdapterName].
//
// # Permissions
//
// Each operation needs these publish rights for its stream S and durable consumer D, plus subscribe on the reply
// inbox: _INBOX.>, or P.> when nats.Config.InboxPrefix is P. Give each user its own prefix so it cannot read others'
// replies; nats.Config.UserInbox sets P to _INBOX.<username>.
//
//   - Producer: the stream's subjects.
//   - EnsureStream: $JS.API.STREAM.INFO.S, $JS.API.STREAM.CREATE.S and $JS.API.STREAM.UPDATE.S.
//   - Consumer that declares D: $JS.API.CONSUMER.CREATE.S.>, $JS.API.CONSUMER.MSG.NEXT.S.D and $JS.ACK.S.D.>.
//   - Consumer with Bind: $JS.API.CONSUMER.INFO.S.D instead of the create right.
//   - Consumer with Declare: the EnsureStream rights as well.
//   - Replay: $JS.API.CONSUMER.CREATE.S.>, $JS.API.CONSUMER.MSG.NEXT.S.* and $JS.API.CONSUMER.DELETE.S.*, plus
//     $JS.API.STREAM.INFO.S for a time start.
//
// A consumer without a Stream also needs $JS.API.STREAM.NAMES to find it. The server does not answer a request the
// user has no right to send, so a missing right shows up as a timeout after nats.Config.PublishTimeout, which bounds
// every request, and a fetch it drops ends with [ErrFetchIncomplete].
package jetstream
