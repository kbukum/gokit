// Package cleanup deletes expired database rows in bounded batches. Schedule DeleteExpired with worker.TickerWorker; this package starts no goroutines and owns no timers.
package cleanup
