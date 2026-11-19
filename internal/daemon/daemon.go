package daemon

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/RakhaYandra/appscope/internal/engine"
	"github.com/RakhaYandra/appscope/internal/group"
	"github.com/RakhaYandra/appscope/internal/ipc"
	"github.com/RakhaYandra/appscope/internal/store"
)

type Daemon struct {
	Store     *store.Store
	Collector *engine.Collector
	Grouper   *group.Grouper
	Interval  time.Duration
	Alerts    *AlertEvaluator

	state atomic.Value
}

func New(st *store.Store) *Daemon {
	return &Daemon{
		Store:     st,
		Collector: engine.NewCollector(),
		Grouper:   &group.Grouper{},
		Interval:  time.Second,
		Alerts:    newAlertEvaluator(st),
	}
}

func (d *Daemon) Tick() error {
	sample, err := d.Collector.Sample()
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	apps := d.Grouper.Group(sample.Processes)

	if err := d.Store.InsertSystemMetrics(sample.System); err != nil {
		return fmt.Errorf("persist system metrics: %w", err)
	}
	if err := d.Store.InsertAppMetrics(apps); err != nil {
		return fmt.Errorf("persist app metrics: %w", err)
	}

	d.state.Store(ipc.Snapshot{
		Timestamp: sample.Timestamp,
		System:    sample.System,
		Apps:      apps,
	})
	if d.Alerts != nil {
		d.Alerts.Evaluate(apps)
	}
	return nil
}

func (d *Daemon) Snapshot() ipc.Snapshot {
	v := d.state.Load()
	if v == nil {
		return ipc.Snapshot{Timestamp: time.Now()}
	}
	return v.(ipc.Snapshot)
}

func (d *Daemon) Run(ctx context.Context) error {
	if d.Interval <= 0 {
		d.Interval = time.Second
	}
	if err := d.Tick(); err != nil {
		return err
	}
	_, _ = d.Store.Prune(time.Now().Add(-7 * 24 * time.Hour))

	collectorTicker := time.NewTicker(d.Interval)
	defer collectorTicker.Stop()
	pruneTicker := time.NewTicker(24 * time.Hour)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-collectorTicker.C:
			_ = d.Tick() // a failed tick must not kill the collector (NFR06)
		case <-pruneTicker.C:
			_, _ = d.Store.Prune(time.Now().Add(-7 * 24 * time.Hour))
		}
	}
}
