package tunnel_test

import (
	"context"
	"fmt"
)

type runnerCall struct {
	name string
	args []string
}

type runnerResult struct {
	stdout []byte
	stderr []byte
	err    error
}

type fakeRunner struct {
	calls   []runnerCall
	results []runnerResult
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nil, nil, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result.stdout, result.stderr, result.err
}

func stateJSON(backend, dnsName string) []byte {
	return []byte(fmt.Sprintf(`{"BackendState":%q,"Self":{"DNSName":%q}}`, backend, dnsName))
}

func funnelJSON(hostport, path, proxy string, allow bool) []byte {
	return []byte(fmt.Sprintf(
		`{"TCP":{"443":{"HTTPS":true}},"Web":{%q:{"Handlers":{%q:{"Proxy":%q}}}},"AllowFunnel":{%q:%t}}`,
		hostport, path, proxy, hostport, allow,
	))
}
