package lottery

import (
	"context"
	"log"
	"sync"
	"time"
)

const rewardWorkerInterval = 30 * time.Second

type Worker struct {
	service *Service
	stop    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

func NewWorker(service *Service) *Worker { return &Worker{service: service, stop: make(chan struct{})} }

func (w *Worker) Start(ctx context.Context) {
	w.wg.Go(func() {
		w.loop(ctx)
	})
}
func (w *Worker) Stop() { w.once.Do(func() { close(w.stop) }) }
func (w *Worker) Wait() { w.wg.Wait() }

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(rewardWorkerInterval)
	defer ticker.Stop()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	defer func() {
		if value := recover(); value != nil {
			log.Printf("[lottery] worker recovered panic=%v", value)
		}
	}()
	w.service.ProcessRewardJobs(ctx, 5)
	w.service.ProcessRateCleanupJobs(ctx, 5)
}
