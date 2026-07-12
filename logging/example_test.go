package logging_test

import (
	"fmt"

	"github.com/kbukum/gokit/logging"
)

func ExampleFields() {
	f := logging.Fields("user_id", 42, "tenant", "acme")
	fmt.Println(f["user_id"], f["tenant"])
	// Output: 42 acme
}

func ExampleErrorFields() {
	f := logging.ErrorFields("save_user", fmt.Errorf("db offline"))
	fmt.Println(f["operation"], f["error"])
	// Output: save_user db offline
}
