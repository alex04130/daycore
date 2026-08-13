package channels

import (
	"context"
	"testing"
)

type fakeChannel struct {
	name string
}

func (f *fakeChannel) Name() string                                   { return f.name }
func (f *fakeChannel) Send(context.Context, string, Outbound) error   { return nil }
func (f *fakeChannel) Features() Features                             { return Features{Markdown: true} }
func (f *fakeChannel) Start(context.Context, chan<- InboundMsg) error { return nil }
func (f *fakeChannel) Stop() error                                    { return nil }

func TestTextHelper(t *testing.T) {
	if got := Text("hi"); got.Text != "hi" || len(got.Attachments) != 0 {
		t.Errorf("Text(%q) = %+v", "hi", got)
	}
}

func TestRegistryRegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&fakeChannel{name: "onebot"})
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("registering the same channel name twice must panic — a silently shadowed adapter keeps running and becomes unreachable")
		}
	}()
	r.Register(&fakeChannel{name: "onebot"})
}

func TestRegistryNilLoggerDoesNotPanic(t *testing.T) {
	r := NewRegistry(nil)
	// Register must not dereference a nil logger.
	r.Register(&fakeChannel{name: "a"})
}

func TestRegistryLookupAndInbound(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(&fakeChannel{name: "onebot"})
	if r.Channel("onebot") == nil {
		t.Fatal("registered channel must be found by name")
	}
	if r.Channel("wechat") != nil {
		t.Fatal("an unregistered name must return nil")
	}
	if r.Inbound() == nil {
		t.Fatal("Inbound must always return a channel")
	}
}
