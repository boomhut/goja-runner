package jsrunner_test

import (
	"fmt"
	"sync"

	jsrunner "github.com/boomhut/goja-runner"
)

// SharedCounter demonstrates thread-safe state sharing across multiple runners
type SharedCounter struct {
	mu    sync.Mutex
	count int
	log   []string
}

func (sc *SharedCounter) Increment() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.count++
	return sc.count
}

func (sc *SharedCounter) GetCount() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.count
}

func (sc *SharedCounter) AddLog(message string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.log = append(sc.log, message)
}

func (sc *SharedCounter) GetLogs() []string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	result := make([]string, len(sc.log))
	copy(result, sc.log)
	return result
}

func ExampleNewWithGlobals_concurrentWorkers() {
	// Create shared state with proper synchronization
	counter := &SharedCounter{
		count: 0,
		log:   make([]string, 0),
	}

	// Define shared globals that all runners will access
	globals := map[string]interface{}{
		"counter":  counter,
		"apiKey":   "shared-secret-123",
		"maxRetry": 3,
	}

	// JavaScript code that each worker will execute
	workerScript := `
		function processTask(taskId) {
			// Access shared counter
			var current = counter.Increment();
			
			// Log the processing
			counter.AddLog("Task " + taskId + " processed by worker, count: " + current);
			
			// Use shared config
			if (current > maxRetry) {
				return "Task " + taskId + " completed with apiKey: " + apiKey;
			}
			return "Task " + taskId + " in progress";
		}
	`

	const numWorkers = 5
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Launch concurrent workers, each with its own runner
	for i := 0; i < numWorkers; i++ {
		go func(workerId int) {
			defer wg.Done()

			// Each goroutine gets its own runner but shares the same globals
			runner := jsrunner.NewWithGlobals(globals)

			// Load the worker script
			err := runner.LoadScriptString(workerScript)
			if err != nil {
				fmt.Printf("Worker %d failed to load script: %v\n", workerId, err)
				return
			}

			// Process multiple tasks
			for taskNum := 0; taskNum < 3; taskNum++ {
				taskId := fmt.Sprintf("W%d-T%d", workerId, taskNum)
				result, err := runner.Call("processTask", taskId)
				if err != nil {
					fmt.Printf("Worker %d task failed: %v\n", workerId, err)
					continue
				}
				fmt.Printf("Worker %d: %s\n", workerId, jsrunner.ExportString(result))
			}
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Check final state
	fmt.Printf("\nFinal count: %d\n", counter.GetCount())
	fmt.Printf("Total log entries: %d\n", len(counter.GetLogs()))

	// Output will vary due to concurrent execution, but count should be 15
	// and there should be 15 log entries
}

func ExampleNewWithGlobals_sharedCache() {
	// Demonstrates a shared cache accessible from multiple runners
	type Cache struct {
		mu   sync.RWMutex
		data map[string]string
	}

	cache := &Cache{
		data: make(map[string]string),
	}

	// Create cache methods
	cacheGet := func(key string) string {
		cache.mu.RLock()
		defer cache.mu.RUnlock()
		return cache.data[key]
	}

	cacheSet := func(key, value string) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		cache.data[key] = value
	}

	globals := map[string]interface{}{
		"cacheGet": cacheGet,
		"cacheSet": cacheSet,
	}

	// First runner sets values
	runner1 := jsrunner.NewWithGlobals(globals)
	runner1.LoadScriptString(`
		cacheSet("user:1", "Alice");
		cacheSet("user:2", "Bob");
	`)

	// Second runner reads values
	runner2 := jsrunner.NewWithGlobals(globals)
	runner2.LoadScriptString(`
		function getUser(id) {
			return cacheGet("user:" + id);
		}
	`)

	result, _ := runner2.Call("getUser", 1)
	fmt.Println(jsrunner.ExportString(result))

	result, _ = runner2.Call("getUser", 2)
	fmt.Println(jsrunner.ExportString(result))

	// Output:
	// Alice
	// Bob
}
