# NATS JetStream integration fixture

`StartJetStream(ctx, t.TempDir())` starts the pinned official NATS server image `JetStreamImage` with JetStream enabled and one random user, and returns once an authenticated JetStream request succeeds. Docker is required; missing infrastructure fails rather than skipping.

Use `Config()` with `jetstream.New`. It returns a loopback plaintext URL with `AllowInsecureDev`, the username and a `PasswordFile` in the caller's directory. Register `Close` with test cleanup whenever `StartJetStream` returns a non-nil fixture, before checking its error. Provisioning is bounded at two minutes, readiness at twenty seconds and teardown at a fresh thirty seconds. The caller owns the temporary directory.

Only the client port is published, on host loopback. The container runs as the calling user with a read-only root filesystem, dropped capabilities, no privilege escalation, one CPU and 256 MiB memory. Stream data lives on a 256 MiB in-memory filesystem, so it does not survive the container.

`WithUser(name, permissions)` adds a user limited by a NATS permissions block, and `ConfigFor(name)` returns its settings. `Restart` stops and starts the server on the same address; stream data does not survive.

The suite proves publish acknowledgement and deduplication, size and subject refusal, durable redelivery after a nak, replay from a sequence and a time, password refusal, recovery after a broker restart, and the rights each operation needs:

```sh
GOWORK=off go test -tags integration -race -count=1 ./...
```
