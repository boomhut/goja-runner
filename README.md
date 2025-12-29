# goja-runner

[![GoDoc](https://godoc.org/github.com/boomhut/goja-runner?status.svg)](https://godoc.org/github.com/boomhut/goja-runner) [![Go Report Card](https://goreportcard.com/badge/github.com/boomhut/goja-runner)](https://goreportcard.com/report/github.com/boomhut/goja-runner)

A reusable Go package for executing JavaScript code using the [goja](https://github.com/dop251/goja) runtime.

## Features

- Load and execute JavaScript from files or strings
- Call JavaScript functions with Go arguments
- Set global variables in the JavaScript environment
- Export JavaScript results to Go types
- Reuse the same runtime for multiple executions (performance optimization)
- **Event loop with Promise/async-await support** via `EventLoopRunner`
- Built-in `setTimeout`, `setInterval`, and `setImmediate` support

## Installation

```bash
go get github.com/boomhut/goja-runner
```

## Usage

### Basic Example

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/boomhut/goja-runner"
)

func main() {
    // Create a new runner
    runner := jsrunner.New()
    
    // Load JavaScript code
    err := runner.LoadScriptString(`
        function greet(name) {
            return "Hello, " + name + "!";
        }
    `)
    if err != nil {
        log.Fatal(err)
    }
    
    // Call the function
    result, err := runner.Call("greet", "World")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(jsrunner.ExportString(result))
    // Output: Hello, World!
}
```

### Loading from File

```go
runner := jsrunner.New()

// Load a JavaScript file
err := runner.LoadScript("path/to/script.js")
if err != nil {
    log.Fatal(err)
}

// Call functions from the loaded script
result, err := runner.Call("myFunction", arg1, arg2)
```

### Setting Global Variables

```go
runner := jsrunner.New()

// Set globals that will be available in JavaScript
runner.SetGlobal("apiKey", "secret-123")
runner.SetGlobal("config", map[string]interface{}{
    "timeout": 30,
    "retries": 3,
})

runner.LoadScriptString(`
    console.log(apiKey);  // "secret-123"
    console.log(config.timeout);  // 30
`)
```

### Evaluating Expressions

```go
runner := jsrunner.New()

// Evaluate a JavaScript expression
result, err := runner.Eval("2 + 2")
fmt.Println(jsrunner.ExportInt(result))  // 4

// Evaluate complex expressions
result, err = runner.Eval(`
    [1, 2, 3].map(x => x * 2).join(',')
`)
fmt.Println(jsrunner.ExportString(result))  // "2,4,6"
```

### Exporting Values

The package provides helper functions to export JavaScript values to Go types:

```go
// Export to string
str := jsrunner.ExportString(result)

// Export to int64
num := jsrunner.ExportInt(result)

// Export to float64
flt := jsrunner.ExportFloat(result)

// Export to bool
b := jsrunner.ExportBool(result)

// Export to interface{} (auto-detect type)
val := jsrunner.Export(result)
```

### Performance: Reusing the Runner

For better performance, create the runner once and reuse it for multiple operations:

```go
// Initialize once (one-time cost)
runner := jsrunner.New()
err := runner.LoadScript("expensive-bundle.js")  // Load once

// Reuse many times (fast)
for i := 0; i < 1000; i++ {
    result, err := runner.Call("processData", data[i])
    // Process result...
}
```

This avoids reloading and re-parsing the JavaScript code on each execution.

### Concurrency

`Runner` is not safe for concurrent use. The underlying goja runtime must only be accessed by one goroutine at a time. If you need parallel execution, create a separate `Runner` per goroutine or worker and load your scripts into each instance.

#### Sharing State Across Runners

To share state between multiple runners (e.g., in concurrent goroutines), use `NewWithGlobals` and pass pointers to shared Go objects. Ensure proper synchronization with `sync.Mutex`, channels, or atomics to avoid data races.

```go
import (
    "fmt"
    "sync"
    "github.com/boomhut/goja-runner"
)

type SharedCounter struct {
    mu    sync.Mutex
    count int
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

func main() {
    counter := &SharedCounter{count: 0}
    
    // Create callable functions that JS can invoke
    globals := map[string]interface{}{
        "increment": counter.Increment,
        "getCount":  counter.GetCount,
    }
    
    var wg sync.WaitGroup
    wg.Add(3)
    
    // Launch 3 concurrent workers
    for i := 0; i < 3; i++ {
        go func(id int) {
            defer wg.Done()
            
            // Each goroutine gets its own runner
            runner := jsrunner.NewWithGlobals(globals)
            runner.LoadScriptString(`
                function work() {
                    return increment();
                }
            `)
            
            // Each worker increments 100 times
            for j := 0; j < 100; j++ {
                runner.Call("work")
            }
        }(i)
    }
    
    wg.Wait()
    
    // All goroutines safely modified the shared counter
    runner := jsrunner.NewWithGlobals(globals)
    result, _ := runner.Eval("getCount()")
    fmt.Println(jsrunner.ExportInt(result)) // Output: 300
}
```

Note: JavaScript variables themselves are not shared—only the underlying Go objects they reference. Each runner maintains its own JavaScript environment, but all runners can call the same Go functions which safely modify shared state.

### Web Access Helpers

`jsrunner.WithWebAccess` enables built-in fetch helpers so JavaScript can synchronously pull remote resources. Pass an optional `WebAccessConfig` to tweak the timeout or supply a custom `http.Client`.

```go
import (
    "fmt"
    "time"

    jsrunner "github.com/boomhut/goja-runner"
)

runner := jsrunner.New(jsrunner.WithWebAccess(&jsrunner.WebAccessConfig{
    Timeout: 5 * time.Second,
}))

txtResult, _ := runner.Call("fetchText", "https://httpbin.org/get")
fmt.Println(jsrunner.ExportString(txtResult))

jsonResult, _ := runner.Call("fetchJSON", "https://httpbin.org/json")
fmt.Printf("got %#v\n", jsrunner.Export(jsonResult))
```

`fetchText` returns the response body as a string while `fetchJSON` unmarshals JSON into Go values. Because the helpers run inside Go, you retain control over headers, retries, and timeouts even when the script requests external endpoints.

### Event Loop and Promises

For JavaScript code that uses Promises, async/await, `setTimeout`, `setInterval`, or `setImmediate`, use `EventLoopRunner`. This runner wraps the goja runtime with a proper event loop that processes asynchronous callbacks.

#### Basic Promise Example

```go
import (
    "fmt"
    "log"

    jsrunner "github.com/boomhut/goja-runner"
)

func main() {
    runner := jsrunner.NewEventLoopRunner()
    
    // Start the event loop in the background
    runner.Start()
    defer runner.Stop()
    
    // Await a promise
    result, err := runner.AwaitPromise(`
        new Promise(function(resolve) {
            setTimeout(function() {
                resolve("Hello from a Promise!");
            }, 100);
        })
    `)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(result) // Output: Hello from a Promise!
}
```

#### Async/Await Example

```go
runner := jsrunner.NewEventLoopRunner()
runner.Start()
defer runner.Stop()

result, err := runner.AwaitPromise(`
    (async function() {
        await new Promise(resolve => setTimeout(resolve, 50));
        const x = 10;
        await new Promise(resolve => setTimeout(resolve, 50));
        const y = 20;
        return x + y;
    })()
`)
fmt.Println(result) // Output: 30
```

#### Promise Chains

```go
runner := jsrunner.NewEventLoopRunner()
runner.Start()
defer runner.Stop()

result, err := runner.AwaitPromise(`
    Promise.resolve(5)
        .then(x => x * 2)
        .then(x => x + 3)
        .then(x => "Result: " + x)
`)
fmt.Println(result) // Output: Result: 13
```

#### Using Go Functions in Promises

```go
runner := jsrunner.NewEventLoopRunner()

// Set Go functions that JavaScript can call
runner.SetGlobal("processData", func(data string) string {
    return "Processed: " + data
})

runner.Start()
defer runner.Stop()

result, err := runner.AwaitPromise(`
    Promise.resolve("test data")
        .then(data => processData(data))
`)
fmt.Println(result) // Output: Processed: test data
```

#### Scheduling Callbacks from Go

```go
runner := jsrunner.NewEventLoopRunner()
runner.Start()
defer runner.Stop()

// Schedule a timeout from Go
runner.SetTimeout(func(vm *goja.Runtime) {
    vm.RunString(`console.log("Timeout fired!")`)
}, 100*time.Millisecond)

// Schedule a repeating interval from Go
interval := runner.SetInterval(func(vm *goja.Runtime) {
    vm.RunString(`console.log("Interval tick")`)
}, 50*time.Millisecond)

time.Sleep(200 * time.Millisecond)
runner.ClearInterval(interval)
```

#### Direct VM Access

For more control, use `Run` to execute code with direct access to the goja runtime:

```go
runner := jsrunner.NewEventLoopRunner()

runner.Run(func(vm *goja.Runtime) {
    vm.Set("counter", 0)
    vm.RunString(`
        function increment() {
            counter++;
            return counter;
        }
    `)
    result, _ := vm.RunString("increment()")
    fmt.Println(result.ToInteger()) // Output: 1
})
```

### ReactApp Helper (SSR + Hydration)

`ReactApp` bundles a React server entry and matching client entry with [esbuild](https://pkg.go.dev/github.com/evanw/esbuild/pkg/api), injects any required polyfills, and exposes helpers for rendering props on the server and serving the browser bundle.

```go
renderer, err := jsrunner.NewReactApp(jsrunner.ReactAppOptions{
    RunnerOptions: []jsrunner.Option{
        jsrunner.WithWebAccess(&jsrunner.WebAccessConfig{Timeout: 5 * time.Second}),
    },
    Polyfills:   []string{string(polyfillsJS)},
    SSREntry:    serverSource,  // must export renderApp(props)
    ClientEntry: clientSource,  // boots hydrateRoot() (or similar)
})

markup, _ := renderer.Render(map[string]interface{}{"message": "hi"})
bundle := renderer.ClientBundle()
```

`ReactApp` compiles both entries in-memory, so you do not need Node.js or a separate build step. Provide your own source strings or load them from disk/templates.

### Example: React SSR with Fiber

The [`examples/fiber-react`](examples/fiber-react) sample wires `ReactApp` into a Fiber server. On boot, `ReactApp` downloads `react`, `react-dom/server`, and `react-dom/client` from [esm.sh](https://esm.sh), bundles the provided server/client entries, and exposes helpers to render HTML and serve the browser bundle.

Steps:

1. Ensure the machine can reach `https://esm.sh` (the first run caches the React bundles locally in memory).
2. Run `go run ./examples/fiber-react`.
3. Visit `http://localhost:3000` to see the SSR output and client hydration.

Key parts of the example:

```go
func main() {
    renderer, err := jsrunner.NewReactApp(jsrunner.ReactAppOptions{
        RunnerOptions: []jsrunner.Option{
            jsrunner.WithWebAccess(&jsrunner.WebAccessConfig{Timeout: 5 * time.Second}),
        },
        Polyfills:   []string{string(polyfills)},
        SSREntry:    defaultSSREntry,
        ClientEntry: defaultClientEntry,
    })
    if err != nil {
        log.Fatalf("boot renderer: %v", err)
    }

    app := fiber.New()

    app.Get("/", func(c *fiber.Ctx) error {
        reqStart := time.Now()
        props := map[string]interface{}{
            "user":      "Fiber",
            "timestamp": time.Now().Format(time.RFC3339),
            "message":   "Hello from goja-runner + React",
            "bundleMs":  millis(renderer.BundleDuration()),
            "metrics":   renderer.MetricsSnapshot(),
        }

        renderStart := time.Now()
        markup, err := renderer.Render(props)
        if err != nil {
            return fiber.NewError(fiber.StatusInternalServerError, err.Error())
        }

        renderDuration := time.Since(renderStart)
        requestDuration := time.Since(reqStart)
        renderer.RecordMetrics(renderDuration, requestDuration)

        html := fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>goja-runner + React SSR</title>
    <style>%s</style>
  </head>
  <body>
    <div id="root">%s</div>
    <script>window.__INITIAL_PROPS__ = %s;</script>
    <script src="/static/client.bundle.js"></script>
  </body>
</html>`, pageStyles, markup, mustJSON(props))

        c.Set("Server-Timing", fmt.Sprintf("render;dur=%.2f,request;dur=%.2f",
            millis(renderDuration), millis(requestDuration)))
        return c.Type("html").SendString(html)
    })

    app.Get("/static/client.bundle.js", func(c *fiber.Ctx) error {
        c.Type("js")
        return c.SendString(renderer.ClientBundle())
    })

    app.Get("/metrics", func(c *fiber.Ctx) error {
        c.Set("Cache-Control", "no-store, max-age=0")
        return c.JSON(fiber.Map{"metrics": renderer.MetricsSnapshot()})
    })

    log.Println("listening on http://localhost:3000")
    log.Fatal(app.Listen(":3000"))
}
```

`millis`/`pageStyles` in the example are small helpers that convert a `time.Duration` into milliseconds and embed the CSS used by the React components.

When the process boots it runs esbuild twice—once targeting the server runtime (to create an IIFE that populates `globalThis.renderApp`) and once for the browser bundle (hydration). Because everything stays inside Go, the example remains self-contained while still benefiting from a proper React/JSX toolchain. The enhanced example also captures per-request render timings, emits `Server-Timing` headers, serves a `/metrics` JSON endpoint, and hydrates a React dashboard that can refresh those metrics with `fetch`. `ReactApp` handles the bundling glue so your application code only needs to provide the two entry strings and wire the results into your HTTP framework of choice.

### Exposing Network Helpers to JavaScript

Workers cannot perform HTTP requests directly inside goja, but you can expose Go helpers that wrap `net/http` (or any other client) and make them available via `SetGlobal`. The Fiber+React example registers a `fetchJSON(url string) (map[string]interface{}, error)` helper so scripts can pull data from external APIs before rendering:

```go
httpClient := &http.Client{Timeout: 5 * time.Second}

fetchJSON := func(url string) (map[string]interface{}, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return nil, fmt.Errorf("fetchJSON got status %d", resp.StatusCode)
    }

    var payload map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return nil, err
    }

    return payload, nil
}

runner.SetGlobal("fetchJSON", fetchJSON)
```

In JavaScript you can now call `const post = fetchJSON("https://jsonplaceholder.typicode.com/posts/1");` and render it synchronously. By keeping networking logic in Go you retain full control over timeouts, retries, and authentication, while scripts get a simple API.

## API Reference

### Runner

#### `New() *Runner`
Creates a new JavaScript runner with a fresh runtime environment.

#### `NewWithGlobals(globals map[string]interface{}) *Runner`
Creates a new JavaScript runner with the provided global variables pre-set. Useful for sharing state across multiple runner instances.

#### `SetGlobal(name string, value interface{})`
Sets a global variable in the JavaScript environment.

#### `LoadScript(filepath string) error`
Loads and executes a JavaScript file.

#### `LoadScriptString(code string) error`
Loads and executes JavaScript code from a string.

#### `Call(functionName string, args ...interface{}) (goja.Value, error)`
Calls a JavaScript function with the provided arguments.

#### `Eval(expression string) (goja.Value, error)`
Evaluates a JavaScript expression and returns the result.

#### `GetVM() *goja.Runtime`
Returns the underlying goja.Runtime for advanced usage.

### EventLoopRunner

#### `NewEventLoopRunner(opts ...Option) *EventLoopRunner`
Creates a new event loop runner with support for Promises, async/await, setTimeout, setInterval, and setImmediate.

#### `NewEventLoopRunnerWithGlobals(globals map[string]interface{}, opts ...Option) *EventLoopRunner`
Creates a new event loop runner with predefined global variables.

#### `Start()`
Starts the event loop in the background. Must be called before using `AwaitPromise`, `SetTimeout`, or `SetInterval`.

#### `Stop()`
Stops the event loop and waits for all pending callbacks to complete.

#### `StopNoWait()`
Stops the event loop immediately without waiting for pending callbacks.

#### `SetGlobal(name string, value interface{})`
Sets a global variable (thread-safe).

#### `Run(fn func(*goja.Runtime))`
Executes code synchronously with direct access to the goja runtime.

#### `RunAsync(code string) (goja.Value, error)`
Executes JavaScript code and waits for all promises and timers to complete.

#### `RunAsyncWithTimeout(code string, timeout time.Duration) (goja.Value, error)`
Executes JavaScript code with a timeout.

#### `AwaitPromise(code string) (interface{}, error)`
Executes JavaScript code that returns a promise and waits for it to resolve. Returns the resolved value or an error if the promise rejects.

#### `SetTimeout(fn func(*goja.Runtime), delay time.Duration) *eventloop.Timer`
Schedules a Go function to be called after the specified delay.

#### `SetInterval(fn func(*goja.Runtime), interval time.Duration) *eventloop.Interval`
Schedules a Go function to be called repeatedly at the specified interval.

#### `ClearTimeout(t *eventloop.Timer)`
Cancels a timeout before it fires.

#### `ClearInterval(i *eventloop.Interval)`
Cancels an interval.

#### `RunOnLoop(fn func(*goja.Runtime))`
Schedules a function to run on the next iteration of the event loop.

### Helper Functions

- `ExportString(val goja.Value) string`
- `ExportInt(val goja.Value) int64`
- `ExportFloat(val goja.Value) float64`
- `ExportBool(val goja.Value) bool`
- `Export(val goja.Value) interface{}`

## License

MIT License. See [LICENSE](LICENSE) for details.
