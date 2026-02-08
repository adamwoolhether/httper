package download

import (
	"context"
	"sync"
)

type Queue struct {
	wg         sync.WaitGroup
	mu         sync.RWMutex
	sem        chan bool
	isShutdown chan struct{}
	running    map[string]context.CancelFunc
}
