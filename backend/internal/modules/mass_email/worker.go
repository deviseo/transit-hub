package mass_email

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	workerConcurrency = 2
	workerInterval    = 2 * time.Second
	staleSendingAfter = 10 * time.Minute
)

type Worker struct {
	service *Service
	stop    chan struct{}
	once    sync.Once
}

func NewWorker(service *Service) *Worker {
	return &Worker{service: service, stop: make(chan struct{})}
}

func (w *Worker) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *Worker) Stop() {
	w.once.Do(func() { close(w.stop) })
}

func (w *Worker) loop(ctx context.Context) {
	w.service.recoverStaleSending(ctx, staleSendingAfter)
	ticker := time.NewTicker(workerInterval)
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
	w.service.recoverStaleSending(ctx, staleSendingAfter)
	items, err := w.service.repository.ClaimPendingItems(ctx, workerConcurrency)
	if err != nil {
		log.Printf("[mass-email] claim pending items failed err=%v", err)
		return
	}
	var wg sync.WaitGroup
	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.service.processOne(ctx, item)
		}()
	}
	wg.Wait()
}
