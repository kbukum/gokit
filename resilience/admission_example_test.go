package resilience_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kbukum/gokit/resilience"
)

func ExamplePolicy_Acquire() {
	policy := resilience.NewPolicy().
		WithTimeout(time.Second).
		WithBulkhead(resilience.BulkheadConfig{MaxConcurrent: 2})

	err := func() (err error) {
		ctx, finish, err := policy.Acquire(context.Background())
		if err != nil {
			return err
		}
		defer func() { err = finish(err) }()

		// A transport opens its stream with ctx and owns cancellation of blocked I/O.
		body := io.NopCloser(strings.NewReader("model output"))
		defer func() { err = errors.Join(err, body.Close()) }()
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}()
	fmt.Println(err)
	// Output:
	// model output
	// <nil>
}
