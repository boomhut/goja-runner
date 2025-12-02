# goja-runner

[![GoDoc](https://godoc.org/github.com/boomhut/goja-runner?status.svg)](https://godoc.org/github.com/boomhut/goja-runner) [![Go Report Card](https://goreportcard.com/badge/github.com/boomhut/goja-runner)](https://goreportcard.com/report/github.com/boomhut/goja-runner)

A reusable Go package for executing JavaScript code using the [goja](https://github.com/dop251/goja) runtime.

## Features

- Load and execute JavaScript from files or strings
- Call JavaScript functions with Go arguments
- Set global variables in the JavaScript environment
- Export JavaScript results to Go types
- Reuse the same runtime for multiple executions (performance optimization)

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
                props := map[string]interface{}{
                        "user":      "Fiber",
                        "timestamp": time.Now().Format(time.RFC3339),
                        "message":   "Hello from goja-runner + React",
                }

                markup, err := renderer.Render(props)
                if err != nil {
                        return fiber.NewError(fiber.StatusInternalServerError, err.Error())
                }

                html := fmt.Sprintf(`<!doctype html>
<html>
    <head>
        <meta charset="utf-8" />
        <title>goja-runner + React SSR</title>
    </head>
    <body>
        <div id="root">%s</div>
        <script>window.__INITIAL_PROPS__ = %s;</script>
        <script src="/static/client.bundle.js"></script>
    </body>
</html>`, markup, mustJSON(props))

                return c.Type("html").SendString(html)
        })

        app.Get("/static/client.bundle.js", func(c *fiber.Ctx) error {
                c.Type("js")
                return c.SendString(renderer.ClientBundle())
        })

    log.Println("listening on http://localhost:3000")
    log.Fatal(app.Listen(":3000"))
}
```

When the process boots it runs esbuild twice—once targeting the server runtime (to create an IIFE that populates `globalThis.renderApp`) and once for the browser bundle (hydration). Because everything stays inside Go, the example remains self-contained while still benefiting from a proper React/JSX toolchain. `ReactApp` handles the bundling glue so your application code only needs to provide the two entry strings and wire the results into your HTTP framework of choice.

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

### Helper Functions

- `ExportString(val goja.Value) string`
- `ExportInt(val goja.Value) int64`
- `ExportFloat(val goja.Value) float64`
- `ExportBool(val goja.Value) bool`
- `Export(val goja.Value) interface{}`

## License

MIT License. See [LICENSE](LICENSE) for details.
