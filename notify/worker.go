// Copyright 2015 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/types"
)

// notificationRequest represents a request to send notifications through the pipeline.
type notificationRequest struct {
	ctx     context.Context
	logger  *slog.Logger
	alerts  []*types.Alert
	resultC chan NotificationResult
}

// NotificationResult contains the result of processing a notification request.
type NotificationResult struct {
	ctx    context.Context
	alerts []*types.Alert
	err    error
}

// IntegrationWorker is a persistent worker goroutine that processes notifications
// for a single integration. It eliminates the need for goroutines that block on timers
// by using time.AfterFunc for scheduling.
type IntegrationWorker struct {
	// Configuration
	wait   func() time.Duration
	stages []Stage // dedup, retry, setnotifies (excludes wait stage)

	// Communication channels
	requestC chan *notificationRequest
	stopC    chan struct{}

	// Lifecycle management
	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup

	// Logger for this worker
	logger *slog.Logger

	// Stats for monitoring
	mu            sync.Mutex
	queueDepth    int
	lastProcessed time.Time
}

// NewIntegrationWorker creates a new worker for processing notifications.
// The wait function is used to determine how long to wait before processing.
// The stages slice should contain the pipeline stages (dedup, retry, setnotifies)
// but NOT the wait stage, as waiting is handled by the worker itself.
// The queueSize determines the buffer size for the request channel.
func NewIntegrationWorker(
	ctx context.Context,
	wait func() time.Duration,
	stages []Stage,
	queueSize int,
	logger *slog.Logger,
) *IntegrationWorker {
	workerCtx, cancel := context.WithCancel(ctx)

	if queueSize <= 0 {
		queueSize = 10 // Minimum sensible buffer
	}

	w := &IntegrationWorker{
		wait:     wait,
		stages:   stages,
		requestC: make(chan *notificationRequest, queueSize),
		stopC:    make(chan struct{}),
		ctx:      workerCtx,
		cancel:   cancel,
		logger:   logger,
	}

	w.wg.Add(1)
	go w.run()

	return w
}

// Enqueue submits a notification request to the worker.
// It returns a channel that will receive the result when processing is complete.
// If the worker is stopped or the context is cancelled, an error is returned immediately.
func (w *IntegrationWorker) Enqueue(ctx context.Context, logger *slog.Logger, alerts ...*types.Alert) <-chan NotificationResult {
	resultC := make(chan NotificationResult, 1)

	req := &notificationRequest{
		ctx:     ctx,
		logger:  logger,
		alerts:  alerts,
		resultC: resultC,
	}

	w.mu.Lock()
	w.queueDepth++
	w.mu.Unlock()

	select {
	case w.requestC <- req:
		// Successfully enqueued
		return resultC
	case <-w.ctx.Done():
		// Worker is stopped
		resultC <- NotificationResult{
			ctx:    ctx,
			alerts: nil,
			err:    w.ctx.Err(),
		}
		return resultC
	case <-ctx.Done():
		// Request context cancelled before enqueue
		resultC <- NotificationResult{
			ctx:    ctx,
			alerts: nil,
			err:    ctx.Err(),
		}
		return resultC
	}
}

// Stop gracefully shuts down the worker, waiting for in-flight notifications to complete.
func (w *IntegrationWorker) Stop() {
	w.cancel()
	close(w.stopC)
	w.wg.Wait()
}

// run is the main worker loop that processes notification requests.
func (w *IntegrationWorker) run() {
	defer w.wg.Done()

	for {
		select {
		case req := <-w.requestC:
			w.mu.Lock()
			w.queueDepth--
			w.mu.Unlock()

			w.handleRequest(req)

		case <-w.stopC:
			// Drain remaining requests before exiting
			w.drainRequests()
			return
		}
	}
}

// handleRequest processes a single notification request.
func (w *IntegrationWorker) handleRequest(req *notificationRequest) {
	// Check if the request context is already done
	select {
	case <-req.ctx.Done():
		req.resultC <- NotificationResult{
			ctx:    req.ctx,
			alerts: nil,
			err:    req.ctx.Err(),
		}
		return
	default:
	}

	// Wait stage: Use time.AfterFunc to avoid blocking goroutine
	waitDuration := w.wait()

	if waitDuration == 0 {
		// No wait needed, process immediately
		w.processStages(req)
		return
	}

	// Schedule processing after wait duration using time.AfterFunc
	// This doesn't block a goroutine - the timer is managed by the runtime
	done := make(chan struct{})
	timer := time.AfterFunc(waitDuration, func() {
		close(done)
	})

	// Wait for either timer or context cancellation
	select {
	case <-done:
		// Timer expired, proceed with processing
		w.processStages(req)

	case <-req.ctx.Done():
		// Context cancelled, stop the timer and return error
		timer.Stop()
		req.resultC <- NotificationResult{
			ctx:    req.ctx,
			alerts: nil,
			err:    req.ctx.Err(),
		}

	case <-w.ctx.Done():
		// Worker stopped, stop the timer and return error
		timer.Stop()
		req.resultC <- NotificationResult{
			ctx:    req.ctx,
			alerts: nil,
			err:    w.ctx.Err(),
		}
	}
}

// processStages executes the pipeline stages (dedup, retry, setnotifies).
func (w *IntegrationWorker) processStages(req *notificationRequest) {
	w.mu.Lock()
	w.lastProcessed = time.Now()
	w.mu.Unlock()

	ctx := req.ctx
	alerts := req.alerts
	var err error

	// Execute each stage in the pipeline
	for _, stage := range w.stages {
		if len(alerts) == 0 {
			// No alerts left to process
			break
		}

		ctx, alerts, err = stage.Exec(ctx, req.logger, alerts...)
		if err != nil {
			req.resultC <- NotificationResult{
				ctx:    ctx,
				alerts: alerts,
				err:    err,
			}
			return
		}
	}

	// Successfully processed all stages
	req.resultC <- NotificationResult{
		ctx:    ctx,
		alerts: alerts,
		err:    nil,
	}
}

// drainRequests processes any remaining requests in the queue before shutdown.
func (w *IntegrationWorker) drainRequests() {
	for {
		select {
		case req := <-w.requestC:
			w.mu.Lock()
			w.queueDepth--
			w.mu.Unlock()

			// Return error indicating worker is stopped
			req.resultC <- NotificationResult{
				ctx:    req.ctx,
				alerts: nil,
				err:    context.Canceled,
			}

		default:
			// Channel is empty
			return
		}
	}
}

// Stats returns current worker statistics.
func (w *IntegrationWorker) Stats() WorkerStats {
	w.mu.Lock()
	defer w.mu.Unlock()

	return WorkerStats{
		QueueDepth:    w.queueDepth,
		LastProcessed: w.lastProcessed,
	}
}

// WorkerStats contains statistics about a worker's current state.
type WorkerStats struct {
	QueueDepth    int
	LastProcessed time.Time
}
