package fake

import (
	"context"
	"sync"

	"github.com/Tragidra/logstruct/internal/llm"
)

// Provider is a test double that returns canned responses.
type Provider struct {
	mu       sync.Mutex
	calls    []llm.Request
	response llm.Response
	err      error
}

// New returns a fake Provider that returns an empty successful response by default
func New() *Provider { return &Provider{} }

// SetResponse configures what the fake returns on the next Complete call.
func (p *Provider) SetResponse(r llm.Response) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.response = r
	p.err = nil
}

// SetError configures the fake to return an error on the next Complete call(s).
func (p *Provider) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// Calls returns all requests received so far.
func (p *Provider) Calls() []llm.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]llm.Request, len(p.calls))
	copy(out, p.calls)
	return out
}

func (p *Provider) Name() string { return "fake" }

func (p *Provider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, req)
	if p.err != nil {
		return llm.Response{}, p.err
	}
	return p.response, nil
}
