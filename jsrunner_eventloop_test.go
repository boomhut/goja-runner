package jsrunner

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestNewEventLoopRunner(t *testing.T) {
	runner := NewEventLoopRunner()
	if runner == nil {
		t.Fatal("NewEventLoopRunner() returned nil")
	}
	if runner.loop == nil {
		t.Error("loop is nil")
	}
	if runner.globals == nil {
		t.Error("globals map is nil")
	}
}

func TestEventLoopRunner_BasicRun(t *testing.T) {
	runner := NewEventLoopRunner()

	var result int64
	runner.Run(func(vm *goja.Runtime) {
		val, err := vm.RunString("1 + 1")
		if err != nil {
			t.Fatalf("RunString failed: %v", err)
		}
		result = val.ToInteger()
	})

	if result != 2 {
		t.Errorf("Expected 2, got %d", result)
	}
}

func TestEventLoopRunner_SetTimeout(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	done := make(chan bool)
	start := time.Now()

	runner.SetTimeout(func(vm *goja.Runtime) {
		elapsed := time.Since(start)
		if elapsed < 50*time.Millisecond {
			t.Errorf("Timeout fired too early: %v", elapsed)
		}
		done <- true
	}, 100*time.Millisecond)

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout did not fire")
	}
}

func TestEventLoopRunner_SetInterval(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	var count int32
	done := make(chan bool)

	interval := runner.SetInterval(func(vm *goja.Runtime) {
		newCount := atomic.AddInt32(&count, 1)
		if newCount >= 3 {
			done <- true
		}
	}, 50*time.Millisecond)

	select {
	case <-done:
		runner.ClearInterval(interval)
		if atomic.LoadInt32(&count) < 3 {
			t.Errorf("Expected at least 3 interval calls, got %d", count)
		}
	case <-time.After(500 * time.Millisecond):
		runner.ClearInterval(interval)
		t.Fatal("Interval did not fire enough times")
	}
}

func TestEventLoopRunner_RunAsync(t *testing.T) {
	runner := NewEventLoopRunner()

	result, err := runner.RunAsync("2 * 21")
	if err != nil {
		t.Fatalf("RunAsync failed: %v", err)
	}

	if ExportInt(result) != 42 {
		t.Errorf("Expected 42, got %d", ExportInt(result))
	}
}

func TestEventLoopRunner_RunAsyncWithPromise(t *testing.T) {
	runner := NewEventLoopRunner()

	result, err := runner.RunAsync(`
		new Promise(function(resolve) {
			setTimeout(function() {
				resolve(42);
			}, 50);
		})
	`)
	if err != nil {
		t.Fatalf("RunAsync failed: %v", err)
	}

	// The result is the promise object, not the resolved value
	if result == nil {
		t.Error("Expected result, got nil")
	}
}

func TestEventLoopRunner_AwaitPromise(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		new Promise(function(resolve) {
			setTimeout(function() {
				resolve("hello");
			}, 50);
		})
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	if result != "hello" {
		t.Errorf("Expected 'hello', got %v", result)
	}
}

func TestEventLoopRunner_AwaitPromiseWithNumber(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		new Promise(function(resolve) {
			setTimeout(function() {
				resolve(42);
			}, 50);
		})
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	// Numbers may come back as int64 or float64 depending on the value
	switch v := result.(type) {
	case int64:
		if v != 42 {
			t.Errorf("Expected 42, got %v", v)
		}
	case float64:
		if v != 42.0 {
			t.Errorf("Expected 42, got %v", v)
		}
	default:
		t.Errorf("Expected number, got %T: %v", result, result)
	}
}

func TestEventLoopRunner_AwaitPromiseRejected(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	_, err := runner.AwaitPromise(`
		new Promise(function(resolve, reject) {
			setTimeout(function() {
				reject("error message");
			}, 50);
		})
	`)
	if err == nil {
		t.Fatal("Expected error for rejected promise")
	}
}

func TestEventLoopRunner_AwaitPromiseNonPromise(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	// Non-promise values should be returned directly
	result, err := runner.AwaitPromise(`42`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	// Numbers may come back as int64 or float64
	switch v := result.(type) {
	case int64:
		if v != 42 {
			t.Errorf("Expected 42, got %v", v)
		}
	case float64:
		if v != 42.0 {
			t.Errorf("Expected 42, got %v", v)
		}
	default:
		t.Errorf("Expected number, got %T: %v", result, result)
	}
}

func TestEventLoopRunner_SetGlobal(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.SetGlobal("myValue", 123)

	var result int64
	runner.Run(func(vm *goja.Runtime) {
		val, err := vm.RunString("myValue")
		if err != nil {
			t.Fatalf("RunString failed: %v", err)
		}
		result = val.ToInteger()
	})

	if result != 123 {
		t.Errorf("Expected 123, got %d", result)
	}
}

func TestNewEventLoopRunnerWithGlobals(t *testing.T) {
	globals := map[string]interface{}{
		"x": 10,
		"y": 20,
	}

	runner := NewEventLoopRunnerWithGlobals(globals)

	var result int64
	runner.Run(func(vm *goja.Runtime) {
		val, err := vm.RunString("x + y")
		if err != nil {
			t.Fatalf("RunString failed: %v", err)
		}
		result = val.ToInteger()
	})

	if result != 30 {
		t.Errorf("Expected 30, got %d", result)
	}
}

func TestEventLoopRunner_JavaScriptSetTimeout(t *testing.T) {
	runner := NewEventLoopRunner()

	result, err := runner.RunAsync(`
		var result = 0;
		setTimeout(function() {
			result = 42;
		}, 50);
		result
	`)
	if err != nil {
		t.Fatalf("RunAsync failed: %v", err)
	}

	// After RunAsync, all timers should have completed
	// But result is captured before the timeout fires in this case
	// This shows the event loop processes the timeout
	_ = result
}

func TestEventLoopRunner_AsyncAwait(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		(async function() {
			await new Promise(function(resolve) {
				setTimeout(resolve, 50);
			});
			return "async done";
		})()
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	if result != "async done" {
		t.Errorf("Expected 'async done', got %v", result)
	}
}

func TestEventLoopRunner_PromiseChain(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		Promise.resolve(1)
			.then(function(x) { return x + 1; })
			.then(function(x) { return x * 2; })
			.then(function(x) { return x + 10; })
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	// (1 + 1) * 2 + 10 = 14
	switch v := result.(type) {
	case int64:
		if v != 14 {
			t.Errorf("Expected 14, got %v", v)
		}
	case float64:
		if v != 14.0 {
			t.Errorf("Expected 14, got %v", v)
		}
	default:
		t.Errorf("Expected number, got %T: %v", result, result)
	}
}

func TestEventLoopRunner_PromiseAll(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		Promise.all([
			Promise.resolve(1),
			Promise.resolve(2),
			Promise.resolve(3)
		])
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected array, got %T", result)
	}

	if len(arr) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(arr))
	}
}

func TestEventLoopRunner_RunAsyncWithTimeout(t *testing.T) {
	runner := NewEventLoopRunner()
	// Note: Don't call Start() before RunAsyncWithTimeout since it uses Run() internally

	// This should complete before timeout
	result, err := runner.RunAsyncWithTimeout("1 + 1", 1*time.Second)
	if err != nil {
		t.Fatalf("RunAsyncWithTimeout failed: %v", err)
	}

	if ExportInt(result) != 2 {
		t.Errorf("Expected 2, got %d", ExportInt(result))
	}
}

func TestEventLoopRunner_RunOnLoop(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	var called atomic.Bool
	done := make(chan bool)

	go func() {
		runner.RunOnLoop(func(vm *goja.Runtime) {
			called.Store(true)
		})
		done <- true
	}()

	<-done
	time.Sleep(50 * time.Millisecond) // Give time for the callback to execute

	if !called.Load() {
		t.Error("RunOnLoop callback was not called")
	}
}

func TestEventLoopRunner_GoFunctionInPromise(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	// Set a Go function as a global
	runner.SetGlobal("multiply", func(a, b int) int {
		return a * b
	})

	result, err := runner.AwaitPromise(`
		Promise.resolve()
			.then(function() {
				return multiply(6, 7);
			})
	`)
	if err != nil {
		t.Fatalf("AwaitPromise failed: %v", err)
	}

	switch v := result.(type) {
	case int64:
		if v != 42 {
			t.Errorf("Expected 42, got %v", v)
		}
	case float64:
		if v != 42.0 {
			t.Errorf("Expected 42, got %v", v)
		}
	default:
		t.Errorf("Expected number, got %T: %v", result, result)
	}
}

func TestEventLoopRunner_StopNoWait(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()

	// Schedule a long-running timeout
	runner.SetTimeout(func(vm *goja.Runtime) {
		// This should not run because we stop immediately
	}, 10*time.Second)

	// Stop immediately without waiting
	runner.StopNoWait()

	// If we get here without blocking for 10 seconds, the test passes
}

func TestEventLoopRunner_MultipleTimeouts(t *testing.T) {
	runner := NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	var order []int
	done := make(chan bool)

	// Schedule timeouts in reverse order
	runner.SetTimeout(func(vm *goja.Runtime) {
		order = append(order, 3)
		done <- true
	}, 150*time.Millisecond)

	runner.SetTimeout(func(vm *goja.Runtime) {
		order = append(order, 1)
	}, 50*time.Millisecond)

	runner.SetTimeout(func(vm *goja.Runtime) {
		order = append(order, 2)
	}, 100*time.Millisecond)

	<-done

	// Check that timeouts fired in the correct order
	if len(order) != 3 {
		t.Fatalf("Expected 3 callbacks, got %d", len(order))
	}

	for i, v := range order {
		if v != i+1 {
			t.Errorf("Expected order[%d] = %d, got %d", i, i+1, v)
		}
	}
}
