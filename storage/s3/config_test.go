package s3

import (
	"strings"
	"testing"
)

// An endpoint is a credential-free S3 origin over HTTPS; plain HTTP needs AllowInsecureDev.
func TestConfigValidatesEndpoint(t *testing.T) {
	t.Parallel()
	valid := func(endpoint string, insecure bool) Config {
		return Config{Bucket: "b", Region: "r", Endpoint: endpoint, AllowInsecureDev: insecure}
	}
	for _, ok := range []Config{
		valid("", false), valid("https://s3.example.com", false), valid("https://s3.example.com:9000/", false),
		valid("http://localhost:8333", true),
	} {
		if err := ok.Validate(); err != nil {
			t.Fatalf("%q: %v", ok.Endpoint, err)
		}
	}
	for _, bad := range []Config{
		valid("http://localhost:8333", false),
		valid("ftp://s3.example.com", true),
		valid("https://user:pass@s3.example.com", false),
		valid("https://s3.example.com/bucket", false),
		valid("https://s3.example.com?x=1", false),
		valid("https://s3.example.com?", false),
		valid("https://s3.example.com#f", false),
		valid("https://s3.example.com.", false),
		valid("https://s3.example.com:", false),
		valid("https://s3.example.com:70000", false),
		valid("https:s3.example.com", false),
		valid("s3.example.com", false),
	} {
		if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "endpoint") {
			t.Fatalf("%q: validate = %v", bad.Endpoint, err)
		}
	}
}
