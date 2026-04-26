package httpclient

import (
	"github.com/kbukum/gokit/provider"
)

// compile-time assertions
var (
	_ provider.RequestResponse[Request, *Response] = (*Adapter)(nil)
	_ provider.Closeable                           = (*Adapter)(nil)
)
