package trace

import "context"

// Recorder persists a retrieval event with its hits.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// RecorderFunc adapts a function to a Recorder.
type RecorderFunc func(ctx context.Context, e Event) error

// Record calls f.
func (f RecorderFunc) Record(ctx context.Context, e Event) error { return f(ctx, e) }

// Nop is a Recorder that keeps nothing.
var Nop Recorder = RecorderFunc(func(context.Context, Event) error { return nil })

// Gated records through next only while enabled reports true, so a setting
// can switch tracing off without the callers knowing.
func Gated(enabled func() bool, next Recorder) Recorder {
	return RecorderFunc(func(ctx context.Context, e Event) error {
		if !enabled() {
			return nil
		}
		return next.Record(ctx, e)
	})
}

// Publishing records through next and then announces the stored event.
func Publishing(publish func(Event), next Recorder) Recorder {
	return RecorderFunc(func(ctx context.Context, e Event) error {
		if err := next.Record(ctx, e); err != nil {
			return err
		}
		publish(e)
		return nil
	})
}
