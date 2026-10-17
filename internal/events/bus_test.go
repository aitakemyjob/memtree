package events

import "testing"

func TestBusFanOutAndCancel(t *testing.T) {
	b := NewBus()
	a, cancelA := b.Subscribe()
	c, cancelC := b.Subscribe()
	defer cancelC()
	b.Publish("x", 1)
	if ev := <-a; ev.Type != "x" || ev.Data != 1 {
		t.Errorf("a got %+v", ev)
	}
	if ev := <-c; ev.Type != "x" {
		t.Errorf("c got %+v", ev)
	}
	cancelA()
	if _, ok := <-a; ok {
		t.Error("cancelled channel should be closed")
	}
	cancelA() // idempotent
	b.Publish("y", nil)
	if ev := <-c; ev.Type != "y" {
		t.Error("remaining subscriber still receives")
	}
}

func TestPublishNeverBlocks(t *testing.T) {
	b := NewBus()
	_, cancel := b.Subscribe()
	defer cancel()
	for i := 0; i < 1000; i++ { // far past the 256 buffer
		b.Publish("flood", i)
	}
}
