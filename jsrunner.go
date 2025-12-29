// Package jsrunner provides a simple and reusable wrapper around the goja JavaScript runtime.
// It enables loading and executing JavaScript code from files or strings, calling JavaScript
// functions from Go, and sharing data between Go and JavaScript environments.
//
// The package is designed for scenarios where you need to:
//   - Execute JavaScript code within a Go application
//   - Call JavaScript functions with Go arguments
//   - Reuse a JavaScript runtime for multiple operations (performance optimization)
//   - Share data between Go and JavaScript contexts
//
// Basic usage:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`function add(a, b) { return a + b; }`)
//	result, err := runner.Call("add", 5, 3)
//	sum := jsrunner.ExportInt(result) // 8
//
// For better performance when executing the same script multiple times,
// initialize the runner once and reuse it:
//
//	runner := jsrunner.New()
//	runner.LoadScript("expensive-bundle.js") // Load once
//	for _, data := range dataset {
//	    result, _ := runner.Call("process", data) // Fast reuse
//	}
package jsrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// Runner represents a JavaScript runtime environment that can execute scripts.
// It maintains a persistent JavaScript VM and tracks global variables that have been set.
// The Runner is safe to reuse for multiple script executions and function calls,
// which can significantly improve performance by avoiding re-parsing and re-initializing
// the JavaScript environment.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("apiKey", "secret-123")
//	runner.LoadScript("script.js")
//	result, err := runner.Call("processData", input)
type Runner struct {
	vm               *goja.Runtime
	globals          map[string]interface{}
	httpClient       *http.Client
	webAccessEnabled bool
	webAccessTimeout time.Duration
}

const defaultWebAccessTimeout = 10 * time.Second

// Option configures Runner behavior during construction.
type Option func(*Runner)

// WebAccessConfig controls the built-in fetch helpers exposed to JavaScript.
type WebAccessConfig struct {
	Client  *http.Client
	Timeout time.Duration
}

// WithWebAccess enables the built-in fetch helpers (`fetchJSON`, `fetchText`).
// Provide a custom HTTP client or timeout via WebAccessConfig; when nil, sensible defaults are used.
func WithWebAccess(cfg *WebAccessConfig) Option {
	return func(r *Runner) {
		r.webAccessEnabled = true
		if cfg == nil {
			return
		}
		if cfg.Client != nil {
			r.httpClient = cfg.Client
		}
		if cfg.Timeout > 0 {
			r.webAccessTimeout = cfg.Timeout
		}
	}
}

func (r *Runner) applyOptions(opts ...Option) {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(r)
	}

	if r.webAccessEnabled {
		r.initWebAccess()
	}
}

// EnableWebAccess turns on the built-in fetch helpers after runner construction.
func (r *Runner) EnableWebAccess(cfg *WebAccessConfig) {
	WithWebAccess(cfg)(r)
	r.webAccessEnabled = true
	r.initWebAccess()
}

func (r *Runner) initWebAccess() {
	if r.webAccessTimeout <= 0 {
		r.webAccessTimeout = defaultWebAccessTimeout
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{Timeout: r.webAccessTimeout}
	}
	r.installFetchGlobals()
}

// New creates and returns a new JavaScript runner with a fresh runtime environment.
// The runner is initialized with an empty global scope and is ready to load scripts
// and execute JavaScript code.
//
// Each call to New creates an isolated JavaScript environment. Scripts loaded in one
// runner do not affect other runners.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`var x = 42;`)
func New(opts ...Option) *Runner {
	runner := &Runner{
		vm:      goja.New(),
		globals: make(map[string]interface{}),
	}
	runner.applyOptions(opts...)
	return runner
}

// NewWithGlobals creates a new JavaScript runner with the provided global variables.
// This is useful for creating multiple runner instances that share the same global state,
// which is particularly helpful when working with concurrent goroutines.
//
// To share mutable state between runners in different goroutines, pass pointers to
// Go objects (e.g., structs, sync.Mutex, channels) and ensure proper synchronization
// to avoid data races.
//
// Example:
//
//	type SharedState struct {
//	    mu      sync.Mutex
//	    counter int
//	}
//
//	state := &SharedState{}
//	globals := map[string]interface{}{
//	    "apiKey": "secret-123",
//	    "state":  state,
//	}
//
//	// Create multiple runners with shared globals
//	runner1 := jsrunner.NewWithGlobals(globals)
//	runner2 := jsrunner.NewWithGlobals(globals)
//
// Note: While the Go-side values are shared, each runner maintains its own
// JavaScript environment. Changes to JavaScript variables in one runner do not
// affect other runners.
func NewWithGlobals(globals map[string]interface{}, opts ...Option) *Runner {
	r := New(opts...)
	for k, v := range globals {
		r.SetGlobal(k, v)
	}
	return r
}

// SetGlobal sets a global variable in the JavaScript environment with the specified name and value.
// The value is stored both in the internal globals map and directly in the JavaScript VM,
// making it accessible to all subsequently executed JavaScript code.
//
// Supported value types include:
//   - Basic types: string, int, float64, bool
//   - Slices and maps (converted to JavaScript arrays and objects)
//   - Structs (fields become JavaScript object properties)
//   - Functions (can be called from JavaScript)
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("apiUrl", "https://api.example.com")
//	runner.SetGlobal("timeout", 30)
//	runner.SetGlobal("debug", true)
//	runner.Eval(`console.log(apiUrl, timeout, debug)`)
func (r *Runner) SetGlobal(name string, value interface{}) {
	r.globals[name] = value
	r.vm.Set(name, value)
}

// LoadScript loads and executes a JavaScript file from the specified filepath.
// The file is read from disk and executed in the runner's JavaScript environment.
// Any global variables, functions, or objects defined in the script become available
// for subsequent operations.
//
// The script is executed immediately upon loading. If the script contains syntax errors
// or runtime errors, they will be returned as an error.
//
// This method is useful for loading external JavaScript libraries or configuration scripts.
//
// Example:
//
//	runner := jsrunner.New()
//	err := runner.LoadScript("./lib/utils.js")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	result, _ := runner.Call("utilFunction", arg)
//
// Returns an error if:
//   - The file cannot be read (e.g., file not found, permission denied)
//   - The JavaScript code contains syntax errors
//   - The JavaScript code throws a runtime error during execution
func (r *Runner) LoadScript(filepath string) error {
	code, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	_, err = r.vm.RunString(string(code))
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}

	return nil
}

// LoadScriptString loads and executes JavaScript code from a string.
// The provided code is executed immediately in the runner's JavaScript environment.
// This is useful for dynamically generated scripts or inline JavaScript code.
//
// Any global variables, functions, or objects defined in the code become available
// for subsequent operations within the same runner instance.
//
// Example:
//
//	runner := jsrunner.New()
//	err := runner.LoadScriptString(`
//	    function greet(name) {
//	        return "Hello, " + name + "!";
//	    }
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	result, _ := runner.Call("greet", "World")
//
// Returns an error if:
//   - The JavaScript code contains syntax errors
//   - The JavaScript code throws a runtime error during execution
func (r *Runner) LoadScriptString(code string) error {
	_, err := r.vm.RunString(code)
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}
	return nil
}

// Call invokes a JavaScript function with the provided arguments.
// The function must be defined in the JavaScript environment (either through LoadScript,
// LoadScriptString, or SetGlobal) before calling.
//
// Arguments are automatically converted from Go types to JavaScript types:
//   - Go strings become JavaScript strings
//   - Go numbers (int, float64, etc.) become JavaScript numbers
//   - Go bools become JavaScript booleans
//   - Go slices become JavaScript arrays
//   - Go maps become JavaScript objects
//
// The result is returned as a goja.Value, which can be converted to Go types using
// the Export helper functions (ExportString, ExportInt, ExportFloat, ExportBool, Export).
//
// Example:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`function add(a, b) { return a + b; }`)
//	result, err := runner.Call("add", 5, 3)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sum := jsrunner.ExportInt(result) // 8
//
// Returns an error if:
//   - The function does not exist in the JavaScript environment
//   - The function throws a runtime error
//   - Arguments cannot be converted to JavaScript types
func (r *Runner) Call(functionName string, args ...interface{}) (goja.Value, error) {
	// Build the function call with arguments
	var jsArgs string
	for i, arg := range args {
		if i > 0 {
			jsArgs += ", "
		}
		// Format the argument based on its type
		switch v := arg.(type) {
		case string:
			jsArgs += fmt.Sprintf("%q", v)
		case int, int32, int64, float32, float64, bool:
			jsArgs += fmt.Sprintf("%v", v)
		default:
			jsArgs += fmt.Sprintf("%v", v)
		}
	}

	script := fmt.Sprintf("%s(%s)", functionName, jsArgs)
	result, err := r.vm.RunString(script)
	if err != nil {
		return nil, fmt.Errorf("failed to call function %s: %w", functionName, err)
	}

	return result, nil
}

// Eval evaluates a JavaScript expression and returns the result.
// This method can execute any valid JavaScript expression, from simple arithmetic
// to complex object manipulations. The expression is evaluated in the context of
// the runner's JavaScript environment, with access to all loaded scripts and globals.
//
// Unlike LoadScript or LoadScriptString, Eval is designed for expressions that
// return a value. For executing statements or definitions, use LoadScriptString instead.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("x", 10)
//	result, err := runner.Eval("x * 2 + 5")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	value := jsrunner.ExportInt(result) // 25
//
// More complex examples:
//
//	// Array operations
//	result, _ := runner.Eval("[1, 2, 3].map(x => x * 2)")
//
//	// Object creation
//	result, _ := runner.Eval("({name: 'John', age: 30})")
//
//	// Accessing properties
//	result, _ := runner.Eval("Math.PI * 2")
//
// Returns an error if:
//   - The expression contains syntax errors
//   - The expression throws a runtime error during evaluation
func (r *Runner) Eval(expression string) (goja.Value, error) {
	result, err := r.vm.RunString(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}
	return result, nil
}

// GetVM returns the underlying goja.Runtime for advanced usage.
// This provides direct access to the JavaScript runtime for operations not covered
// by the Runner's high-level API.
//
// Use this method when you need:
//   - Direct access to goja's low-level APIs
//   - Custom type conversions or object wrapping
//   - Advanced runtime configuration
//   - Integration with other goja-based libraries
//
// Example:
//
//	runner := jsrunner.New()
//	vm := runner.GetVM()
//	// Use vm for advanced goja operations
//	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
//
// Note: Direct manipulation of the VM may affect the Runner's behavior.
// Use with caution and ensure you understand goja's API.
func (r *Runner) GetVM() *goja.Runtime {
	return r.vm
}

func (r *Runner) installFetchGlobals() {
	r.SetGlobal("fetchText", func(url string) (string, error) {
		data, err := r.fetchBytes(url)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})

	r.SetGlobal("fetchJSON", func(url string) (interface{}, error) {
		data, err := r.fetchBytes(url)
		if err != nil {
			return nil, err
		}

		var payload interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}

		return payload, nil
	})
}

func (r *Runner) fetchBytes(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.webAccessTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("fetch request failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ExportString is a helper function that converts a goja.Value to a Go string.
// It handles the conversion of JavaScript values to their string representation.
//
// Conversion behavior:
//   - JavaScript strings are converted directly
//   - Numbers are converted to their string representation
//   - Booleans become "true" or "false"
//   - null becomes "null"
//   - undefined becomes "undefined"
//   - Objects and arrays are converted using their toString() method
//
// If the value is nil, an empty string is returned.
//
// Example:
//
//	result, _ := runner.Eval("'Hello World'")
//	str := jsrunner.ExportString(result) // "Hello World"
//
//	result, _ := runner.Eval("42")
//	str := jsrunner.ExportString(result) // "42"
func ExportString(val goja.Value) string {
	if val == nil {
		return ""
	}
	return val.String()
}

// ExportInt is a helper function that converts a goja.Value to a Go int64.
// It handles the conversion of JavaScript values to integers.
//
// Conversion behavior:
//   - JavaScript numbers are truncated to integers
//   - Strings are parsed as integers if possible
//   - Booleans become 1 (true) or 0 (false)
//   - null and undefined become 0
//   - Non-numeric values become 0
//
// If the value is nil, 0 is returned.
//
// Example:
//
//	result, _ := runner.Eval("42")
//	num := jsrunner.ExportInt(result) // 42
//
//	result, _ := runner.Eval("3.14")
//	num := jsrunner.ExportInt(result) // 3 (truncated)
func ExportInt(val goja.Value) int64 {
	if val == nil {
		return 0
	}
	return val.ToInteger()
}

// ExportFloat is a helper function that converts a goja.Value to a Go float64.
// It handles the conversion of JavaScript values to floating-point numbers.
//
// Conversion behavior:
//   - JavaScript numbers are converted directly
//   - Strings are parsed as floats if possible
//   - Booleans become 1.0 (true) or 0.0 (false)
//   - null and undefined become 0.0
//   - Non-numeric values become 0.0
//
// If the value is nil, 0.0 is returned.
//
// Example:
//
//	result, _ := runner.Eval("3.14159")
//	num := jsrunner.ExportFloat(result) // 3.14159
//
//	result, _ := runner.Eval("42")
//	num := jsrunner.ExportFloat(result) // 42.0
func ExportFloat(val goja.Value) float64 {
	if val == nil {
		return 0
	}
	return val.ToFloat()
}

// ExportBool is a helper function that converts a goja.Value to a Go bool.
// It handles the conversion of JavaScript values to booleans using JavaScript's
// truthy/falsy semantics.
//
// Conversion behavior (JavaScript truthy/falsy rules):
//   - true becomes true
//   - false becomes false
//   - 0, -0, NaN become false
//   - "" (empty string) becomes false
//   - null becomes false
//   - undefined becomes false
//   - All other values become true (including "0", "false", empty arrays/objects)
//
// If the value is nil, false is returned.
//
// Example:
//
//	result, _ := runner.Eval("true")
//	b := jsrunner.ExportBool(result) // true
//
//	result, _ := runner.Eval("1")
//	b := jsrunner.ExportBool(result) // true
//
//	result, _ := runner.Eval("''")
//	b := jsrunner.ExportBool(result) // false
func ExportBool(val goja.Value) bool {
	if val == nil {
		return false
	}
	return val.ToBoolean()
}

// Export is a helper function that converts a goja.Value to a Go interface{}.
// It automatically detects the JavaScript type and converts to the appropriate Go type.
//
// Conversion behavior:
//   - JavaScript strings become Go strings
//   - JavaScript numbers become Go float64
//   - JavaScript booleans become Go bool
//   - JavaScript arrays become Go []interface{}
//   - JavaScript objects become Go map[string]interface{}
//   - JavaScript null becomes Go nil
//   - JavaScript undefined becomes Go nil
//   - JavaScript functions are not directly convertible
//
// If the value is nil, nil is returned.
//
// This function is useful when you don't know the type of the JavaScript value
// in advance or when dealing with dynamic data structures.
//
// Example:
//
//	result, _ := runner.Eval("({name: 'John', age: 30})")
//	obj := jsrunner.Export(result).(map[string]interface{})
//	name := obj["name"].(string) // "John"
//	age := obj["age"].(float64)  // 30.0
//
//	result, _ := runner.Eval("[1, 2, 3]")
//	arr := jsrunner.Export(result).([]interface{})
//	first := arr[0].(float64) // 1.0
func Export(val goja.Value) interface{} {
	if val == nil {
		return nil
	}
	return val.Export()
}

// EventLoopRunner represents a JavaScript runtime with an event loop that supports
// asynchronous operations like Promises, setTimeout, setInterval, and setImmediate.
// It wraps the goja runtime with an event loop for proper async/await support.
//
// Unlike the basic Runner, EventLoopRunner can execute JavaScript code that uses:
//   - Promises and async/await
//   - setTimeout and clearTimeout
//   - setInterval and clearInterval
//   - setImmediate and clearImmediate
//
// The event loop runs in the background and processes callbacks until stopped.
//
// Example:
//
//	runner := jsrunner.NewEventLoopRunner()
//	runner.Start()
//	defer runner.Stop()
//
//	result, err := runner.RunAsync(`
//	    async function fetchData() {
//	        return new Promise(resolve => {
//	            setTimeout(() => resolve("done"), 100);
//	        });
//	    }
//	    fetchData();
//	`)
type EventLoopRunner struct {
	loop             *eventloop.EventLoop
	globals          map[string]interface{}
	mu               sync.RWMutex
	httpClient       *http.Client
	webAccessEnabled bool
	webAccessTimeout time.Duration
}

// NewEventLoopRunner creates a new JavaScript runner with an event loop.
// The event loop provides support for asynchronous JavaScript operations including
// Promises, setTimeout, setInterval, and setImmediate.
//
// The runner must be started with Start() before executing async code,
// and should be stopped with Stop() when done.
//
// Example:
//
//	runner := jsrunner.NewEventLoopRunner()
//	runner.Start()
//	defer runner.Stop()
//
//	result, err := runner.RunAsync(`
//	    new Promise(resolve => setTimeout(() => resolve(42), 100))
//	`)
func NewEventLoopRunner(opts ...Option) *EventLoopRunner {
	r := &EventLoopRunner{
		loop:    eventloop.NewEventLoop(),
		globals: make(map[string]interface{}),
	}
	r.applyOptions(opts...)
	return r
}

// NewEventLoopRunnerWithGlobals creates a new event loop runner with predefined global variables.
// This is useful for sharing state between multiple async operations or providing
// configuration to JavaScript code.
//
// Example:
//
//	globals := map[string]interface{}{
//	    "apiUrl": "https://api.example.com",
//	    "timeout": 5000,
//	}
//	runner := jsrunner.NewEventLoopRunnerWithGlobals(globals)
//	runner.Start()
//	defer runner.Stop()
func NewEventLoopRunnerWithGlobals(globals map[string]interface{}, opts ...Option) *EventLoopRunner {
	r := NewEventLoopRunner(opts...)
	for k, v := range globals {
		r.globals[k] = v
	}
	return r
}

func (r *EventLoopRunner) applyOptions(opts ...Option) {
	// Create a temporary runner to apply options and extract config
	tempRunner := &Runner{
		globals: make(map[string]interface{}),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(tempRunner)
	}

	r.webAccessEnabled = tempRunner.webAccessEnabled
	r.httpClient = tempRunner.httpClient
	r.webAccessTimeout = tempRunner.webAccessTimeout
}

// Start starts the event loop in the background.
// This must be called before using RunAsync, SetTimeout, or SetInterval.
// The event loop will continue running until Stop() is called.
//
// Example:
//
//	runner := jsrunner.NewEventLoopRunner()
//	runner.Start()
//	defer runner.Stop()
func (r *EventLoopRunner) Start() {
	r.loop.Start()
}

// Stop stops the event loop and waits for all pending callbacks to complete.
// After calling Stop(), the runner should not be used again.
//
// Example:
//
//	runner := jsrunner.NewEventLoopRunner()
//	runner.Start()
//	// ... do work ...
//	runner.Stop()
func (r *EventLoopRunner) Stop() {
	r.loop.Stop()
}

// StopNoWait stops the event loop without waiting for pending callbacks.
// Use this when you want to immediately terminate all pending operations.
func (r *EventLoopRunner) StopNoWait() {
	r.loop.StopNoWait()
}

// SetGlobal sets a global variable that will be available in all JavaScript executions.
// This is thread-safe and can be called while the event loop is running.
//
// Example:
//
//	runner.SetGlobal("config", map[string]interface{}{
//	    "debug": true,
//	    "maxRetries": 3,
//	})
func (r *EventLoopRunner) SetGlobal(name string, value interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globals[name] = value
}

// Run executes JavaScript code synchronously within the event loop.
// This is useful for initialization code or synchronous operations.
// The callback receives the goja.Runtime for direct manipulation.
//
// Example:
//
//	runner.Run(func(vm *goja.Runtime) {
//	    vm.Set("myFunc", func(x int) int { return x * 2 })
//	    vm.RunString("var result = myFunc(21);")
//	})
func (r *EventLoopRunner) Run(fn func(*goja.Runtime)) {
	r.loop.Run(func(vm *goja.Runtime) {
		r.setupVM(vm)
		fn(vm)
	})
}

// RunAsync executes JavaScript code and waits for all promises and timers to complete.
// Returns the result of the last expression evaluated.
//
// This method blocks until all asynchronous operations (promises, timeouts, intervals)
// have completed or an error occurs.
//
// Example:
//
//	result, err := runner.RunAsync(`
//	    async function delay(ms) {
//	        return new Promise(resolve => setTimeout(resolve, ms));
//	    }
//
//	    async function main() {
//	        await delay(100);
//	        return "completed";
//	    }
//
//	    main();
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(jsrunner.ExportString(result)) // "completed"
func (r *EventLoopRunner) RunAsync(code string) (goja.Value, error) {
	var result goja.Value
	var runErr error

	r.loop.Run(func(vm *goja.Runtime) {
		r.setupVM(vm)
		result, runErr = vm.RunString(code)
	})

	return result, runErr
}

// RunAsyncWithTimeout executes JavaScript code with a timeout.
// If the code doesn't complete within the specified duration, an error is returned.
//
// Example:
//
//	result, err := runner.RunAsyncWithTimeout(`
//	    new Promise(resolve => setTimeout(() => resolve("done"), 100))
//	`, 5*time.Second)
func (r *EventLoopRunner) RunAsyncWithTimeout(code string, timeout time.Duration) (goja.Value, error) {
	var result goja.Value
	var runErr error
	done := make(chan struct{})

	go func() {
		r.loop.Run(func(vm *goja.Runtime) {
			r.setupVM(vm)
			result, runErr = vm.RunString(code)
		})
		close(done)
	}()

	select {
	case <-done:
		return result, runErr
	case <-time.After(timeout):
		r.loop.StopNoWait()
		return nil, fmt.Errorf("execution timed out after %v", timeout)
	}
}

// AwaitPromise executes JavaScript code that returns a promise and waits for it to resolve.
// The resolved value is returned. If the promise rejects, an error is returned.
//
// Note: The event loop must be started with Start() before calling this method,
// and must NOT be started with Run() (which is blocking).
//
// Example:
//
//	runner.Start()
//	defer runner.Stop()
//	result, err := runner.AwaitPromise(`
//	    fetch("https://api.example.com/data")
//	        .then(response => response.json())
//	`)
func (r *EventLoopRunner) AwaitPromise(code string) (interface{}, error) {
	var resolvedValue interface{}
	var promiseErr error
	done := make(chan struct{})

	r.loop.RunOnLoop(func(vm *goja.Runtime) {
		r.setupVM(vm)

		// Wrap the code to capture the promise result
		wrappedCode := fmt.Sprintf(`
			(function() {
				var __result = { value: undefined, error: undefined, done: false };
				var __promise = %s;
				if (__promise && typeof __promise.then === 'function') {
					__promise.then(function(v) {
						__result.value = v;
						__result.done = true;
					}).catch(function(e) {
						__result.error = e;
						__result.done = true;
					});
				} else {
					__result.value = __promise;
					__result.done = true;
				}
				return __result;
			})()
		`, code)

		result, err := vm.RunString(wrappedCode)
		if err != nil {
			promiseErr = err
			close(done)
			return
		}

		obj := result.ToObject(vm)

		// Set up a check function that will be called after the event loop processes
		var checkResult func()
		checkResult = func() {
			doneVal := obj.Get("done")
			if doneVal.ToBoolean() {
				errorVal := obj.Get("error")
				if !goja.IsUndefined(errorVal) && !goja.IsNull(errorVal) {
					promiseErr = fmt.Errorf("promise rejected: %v", errorVal.Export())
				} else {
					valueVal := obj.Get("value")
					resolvedValue = valueVal.Export()
				}
				close(done)
			} else {
				// Check again on next tick
				r.loop.RunOnLoop(func(vm *goja.Runtime) {
					checkResult()
				})
			}
		}

		// Start checking after the current execution
		r.loop.RunOnLoop(func(vm *goja.Runtime) {
			checkResult()
		})
	})

	<-done
	return resolvedValue, promiseErr
}

// SetTimeout schedules a Go function to be called after the specified duration.
// The callback receives the goja.Runtime for JavaScript execution.
// Returns a timer that can be used to cancel the timeout.
//
// Example:
//
//	runner.SetTimeout(func(vm *goja.Runtime) {
//	    vm.RunString("console.log('Timer fired!')")
//	}, 1*time.Second)
func (r *EventLoopRunner) SetTimeout(fn func(*goja.Runtime), delay time.Duration) *eventloop.Timer {
	return r.loop.SetTimeout(func(vm *goja.Runtime) {
		r.setupVM(vm)
		fn(vm)
	}, delay)
}

// SetInterval schedules a Go function to be called repeatedly at the specified interval.
// The callback receives the goja.Runtime for JavaScript execution.
// Returns an interval that can be passed to ClearInterval to stop it.
//
// Example:
//
//	interval := runner.SetInterval(func(vm *goja.Runtime) {
//	    vm.RunString("counter++")
//	}, 100*time.Millisecond)
//
//	// Later, stop the interval
//	runner.ClearInterval(interval)
func (r *EventLoopRunner) SetInterval(fn func(*goja.Runtime), interval time.Duration) *eventloop.Interval {
	return r.loop.SetInterval(func(vm *goja.Runtime) {
		r.setupVM(vm)
		fn(vm)
	}, interval)
}

// ClearInterval cancels an Interval returned by SetInterval.
// It is safe to call inside or outside the event loop.
//
// Example:
//
//	interval := runner.SetInterval(func(vm *goja.Runtime) {
//	    vm.RunString("tick()")
//	}, 100*time.Millisecond)
//	// ... later ...
//	runner.ClearInterval(interval)
func (r *EventLoopRunner) ClearInterval(i *eventloop.Interval) {
	r.loop.ClearInterval(i)
}

// ClearTimeout cancels a Timer returned by SetTimeout if it has not run yet.
// It is safe to call inside or outside the event loop.
//
// Example:
//
//	timer := runner.SetTimeout(func(vm *goja.Runtime) {
//	    vm.RunString("timeout()")
//	}, 5*time.Second)
//	// Cancel before it fires
//	runner.ClearTimeout(timer)
func (r *EventLoopRunner) ClearTimeout(t *eventloop.Timer) {
	r.loop.ClearTimeout(t)
}

// RunOnLoop schedules a Go function to be executed on the next iteration of the event loop.
// This is useful for executing code that needs to run in the context of the event loop
// from a different goroutine.
//
// Example:
//
//	go func() {
//	    runner.RunOnLoop(func(vm *goja.Runtime) {
//	        vm.RunString("handleExternalEvent()")
//	    })
//	}()
func (r *EventLoopRunner) RunOnLoop(fn func(*goja.Runtime)) {
	r.loop.RunOnLoop(func(vm *goja.Runtime) {
		r.setupVM(vm)
		fn(vm)
	})
}

// setupVM initializes the VM with globals and optional features.
func (r *EventLoopRunner) setupVM(vm *goja.Runtime) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, value := range r.globals {
		vm.Set(name, value)
	}

	if r.webAccessEnabled {
		r.installFetchGlobals(vm)
	}
}

func (r *EventLoopRunner) installFetchGlobals(vm *goja.Runtime) {
	if r.webAccessTimeout <= 0 {
		r.webAccessTimeout = defaultWebAccessTimeout
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{Timeout: r.webAccessTimeout}
	}

	vm.Set("fetchText", func(url string) (string, error) {
		data, err := r.fetchBytes(url)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})

	vm.Set("fetchJSON", func(url string) (interface{}, error) {
		data, err := r.fetchBytes(url)
		if err != nil {
			return nil, err
		}

		var payload interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}

		return payload, nil
	})
}

func (r *EventLoopRunner) fetchBytes(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.webAccessTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("fetch request failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
