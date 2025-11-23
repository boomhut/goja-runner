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

### Example: React SSR with Fiber

You can use goja-runner to server-side render a React application and return the markup through a [Fiber](https://github.com/gofiber/fiber) handler.

1. Download the UMD builds of `react` and `react-dom/server` (e.g., from the official CDN) into `./assets/react.development.js` and `./assets/react-dom-server.development.js`.
2. Bundle your application-specific SSR entry (e.g., with Vite/Rollup) into `./assets/app.ssr.js` and expose a global `renderApp(props)` function that returns `ReactDOMServer.renderToString(<App {...props} />)`.
3. Optionally ship a browser bundle at `./assets/public/client.bundle.js` that hydrates `window.__INITIAL_PROPS__`.

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/gofiber/fiber/v2"
    jsrunner "github.com/boomhut/goja-runner"
)

type ReactRenderer struct {
    mu     sync.Mutex
    runner *jsrunner.Runner
}

func NewReactRenderer() (*ReactRenderer, error) {
    r := jsrunner.New()

    loaders := []struct {
        name string
        fn   func() error
    }{
        {"react", func() error { return r.LoadScript("./assets/react.development.js") }},
        {"react-dom-server", func() error { return r.LoadScript("./assets/react-dom-server.development.js") }},
        {"app", func() error { return r.LoadScript("./assets/app.ssr.js") }},
    }

    for _, loader := range loaders {
        if err := loader.fn(); err != nil {
            return nil, fmt.Errorf("failed to load %s: %w", loader.name, err)
        }
    }

    return &ReactRenderer{runner: r}, nil
}

func (rr *ReactRenderer) Render(props map[string]interface{}) (string, error) {
    rr.mu.Lock()
    defer rr.mu.Unlock()

    rr.runner.SetGlobal("SERVER_PROPS", props)
    result, err := rr.runner.Eval("renderApp(SERVER_PROPS)")
    if err != nil {
        return "", err
    }
    return jsrunner.ExportString(result), nil
}

func mustJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}

func main() {
    renderer, err := NewReactRenderer()
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
    <title>React + goja-runner</title>
  </head>
  <body>
    <div id="root">%s</div>
    <script>window.__INITIAL_PROPS__ = %s;</script>
    <script src="/static/client.bundle.js"></script>
  </body>
</html>`, markup, mustJSON(props))

        return c.Type("html").SendString(html)
    })

    app.Static("/static", "./assets/public")
    log.Fatal(app.Listen(":3000"))
}
```

The renderer uses a mutex to serialize access to the shared goja runtime (Fiber may serve multiple requests concurrently). Each handler injects fresh props, calls the React SSR entry, and returns fully formed HTML plus initial data for hydration on the client.

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
