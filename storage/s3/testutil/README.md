# S3 integration fixture

`StartS3(ctx, t.TempDir())` provisions a pinned SeaweedFS Community Edition node, creates a private random bucket using a separate setup identity, and returns mounted credential paths for a bucket-scoped runtime identity. Docker is required; missing infrastructure fails rather than skipping.

Call `Config()` to construct `s3.NewStorage`, and register `Close` with test cleanup whenever `StartS3` returns a non-nil fixture, before checking its error. Failed provisioning rolls back acquired resources; if rollback fails, the returned fixture retains cleanup ownership for a retry. Provisioning is bounded at three minutes, setup calls at ten seconds, restarts at thirty seconds and teardown at a fresh thirty seconds. The caller owns the temporary directory and must remove it after the container closes. Never log or retain its authentication files.

`Restart` gracefully restarts the same persistent node. `CrashRestart` sends SIGKILL, verifies exit status 137, and starts the same node without replacing its data. Docker may assign a new host port: obtain a fresh configuration and reconstruct clients after either operation. This proves single-process crash recovery, not power-loss durability or a production multi-node topology.

Only the S3 port is published, on host loopback. The non-root container has a read-only root filesystem, dropped capabilities, no privilege escalation, one CPU, 512 MiB memory and a 32 MiB temporary filesystem. Volume/filer state stays in its explicit persistent data mount. Four 64 MiB volume slots bound the intended test workload; they are not a filesystem quota or a guarantee against disk exhaustion. WebDAV, administration UI, IAM mutation, Iceberg, Lance and debug services are disabled; the node still owns required internal master/filer/volume listeners.

The real suite checks effective container constraints and optional listeners, bucket readiness, anonymous refusal, outside-bucket denial, and acknowledged-byte persistence across SIGKILL and graceful restart:

```sh
GOWORK=off go test -tags integration -race -shuffle=on -count=1 ./...
```
