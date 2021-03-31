package server

import (
	"strconv"
	"sync"

	"github.com/rai-project/uuid"
)

type Worker struct {
	ID          string
	Work        chan *WorkRequest
	WorkerQueue chan chan *WorkRequest
	QuitChan    chan bool
}

type Dispatcher struct {
	workers     []*Worker
	workerQueue chan chan *WorkRequest
	workQueue   chan *WorkRequest
	waitgroup   sync.WaitGroup
}

func NewWorker(id int, workerQueue chan chan *WorkRequest) *Worker {
	// Create, and return the worker.
	worker := &Worker{
		ID:          uuid.NewV4() + ":::" + strconv.Itoa(id),
		Work:        make(chan *WorkRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool),
	}

	return worker
}

func (w *Worker) Start() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.WorkerQueue <- w.Work

			select {
			case work := <-w.Work:
				func(w *WorkRequest) {
					defer work.Close()
					// Receive a work request.
					log.Debugf("worker%v: Received work request from \n", w.ID)
					if err := work.Start(); err != nil {
						log.WithError(err).Error("worker: error while working\n")
						return
					}
				}(work)
			case <-w.QuitChan:
				// We have been asked to stop.
				// fmt.Printf("worker-%v stopping\n", w.ID)
				return
			}
		}
	}()
}
func (w *Worker) Stop() {
	close(w.QuitChan)
}

func StartDispatcher(nworkers int) *Dispatcher {
	// First, initialize the channel we are going to but the workers' work channels into.
	workerQueue := make(chan chan *WorkRequest, nworkers)
	workQueue := make(chan *WorkRequest, 100)
	workers := make([]*Worker, nworkers)
	var wg sync.WaitGroup

	// Now, create all of our workers.
	for i := 0; i < nworkers; i++ {
		log.Debug("Starting worker", i+1)
		worker := NewWorker(i+1, workerQueue)
		worker.Start()

		workers[i] = worker
	}

	go func() {
		for work := range workQueue {
			wg.Add(1)
			log.WithField("id", work.ID).Debug("queue work request")
			worker := <-workerQueue
			go func() {
				defer wg.Done()

				log.WithField("id", work.ID).Debug("dispatching work request")
				worker <- work
			}()
		}
	}()
	return &Dispatcher{
		workers:     workers,
		workQueue:   workQueue,
		workerQueue: workerQueue,
		waitgroup:   wg,
	}
}

func (d *Dispatcher) Stop() {
	d.waitgroup.Wait()
	for _, w := range d.workers {
		w.Stop()
	}
}
