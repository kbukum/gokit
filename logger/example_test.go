package logger_test

import (
	"fmt"

	"github.com/kbukum/gokit/logger"
)

func ExampleFields() {
	f := logger.Fields("user_id", 42, "tenant", "acme")
	fmt.Println(f["user_id"], f["tenant"])
	// Output: 42 acme
}

func ExampleErrorFields() {
	f := logger.ErrorFields("save_user", fmt.Errorf("db offline"))
	fmt.Println(f["operation"], f["error"])
	// Output: save_user db offline
}
