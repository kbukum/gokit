package httpclient

import (
	"github.com/kbukum/gokit/provider"
)

// compile-time assertions
var _ provider.RequestResponse[Request, *Response] = (*Adapter)(nil)
var _ provider.Closeable = (*Adapter)(nil)
