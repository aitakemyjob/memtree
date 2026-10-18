package trace

import (
	"context"
	"time"
)

// Retention prunes traces older than a configured number of days.
type Retention struct {
	Pruner Pruner
	Days   func() int // 0 or less keeps everything
	Now    func() time.Time
}

// Run prunes once and returns how many events were removed.
func (r Retention) Run(ctx context.Context) (int, error) {
	d := r.Days()
	if d <= 0 {
		return 0, nil
	}
	return r.Pruner.Prune(ctx, r.Now().AddDate(0, 0, -d))
}
