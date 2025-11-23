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
