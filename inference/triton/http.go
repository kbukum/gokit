package triton

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/httpclient"
)

func (p *Provider) do(ctx context.Context, req httpclient.Request) (*httpclient.Response, error) {
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		if resp != nil && len(resp.Body) > 0 {
			return nil, errors.Join(err, fmt.Errorf("triton: response body: %s", strings.TrimSpace(string(resp.Body))))
		}
		return nil, err
	}
	return resp, nil
}
