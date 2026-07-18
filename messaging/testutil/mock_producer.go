package testutil

// SetError configures MockProducer to return err on all publish operations. Pass nil to clear the injected error. The error takes effect on the next Publish/PublishJSON/PublishBinary/Send call.
func (p *MockProducer) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publishErr = err
}

// HasMessages reports whether the producer recorded any messages.
func (p *MockProducer) HasMessages() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages) > 0
}

// LastMessage returns the most recently recorded message. It panics if no messages have been recorded.
func (p *MockProducer) LastMessage() Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		panic("testutil: LastMessage called on empty MockProducer")
	}
	return p.messages[len(p.messages)-1]
}

// MessageCount returns the total number of recorded messages.
func (p *MockProducer) MessageCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}
