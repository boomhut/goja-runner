package jsrunner_test

import (
	"fmt"
	"log"

	"github.com/dop251/goja"

	jsrunner "github.com/boomhut/goja-runner"
)

func ExampleRunner_basic() {
	// Create a new JavaScript runner
	runner := jsrunner.New()

	// Load a simple script
	err := runner.LoadScriptString(`
		function greet(name) {
			return "Hello, " + name + "!";
		}
		
		function add(a, b) {
			return a + b;
		}
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Call the greet function
	result, err := runner.Call("greet", "World")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(jsrunner.ExportString(result))

	// Call the add function
	sum, err := runner.Call("add", 5, 3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(jsrunner.ExportInt(sum))

	// Output:
	// Hello, World!
	// 8
}

func ExampleRunner_withGlobals() {
	runner := jsrunner.New()

	// Set global variables
	runner.SetGlobal("apiKey", "secret-123")
	runner.SetGlobal("debug", true)

	err := runner.LoadScriptString(`
		function getConfig() {
			return {
				key: apiKey,
				debugMode: debug
			};
		}
	`)
	if err != nil {
		log.Fatal(err)
	}

	result, err := runner.Eval("JSON.stringify(getConfig())")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(jsrunner.ExportString(result))

	// Output:
	// {"key":"secret-123","debugMode":true}
}

func ExampleRunner_loadFromFile() {
	runner := jsrunner.New()

	// Load a JavaScript file
	err := runner.LoadScript("my-script.js")
	if err != nil {
		log.Fatal(err)
	}

	// Call functions from the loaded script
	result, err := runner.Call("myFunction", "arg1", "arg2")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(jsrunner.ExportString(result))
}

func ExampleEventLoopRunner_promises() {
	// Create an event loop runner for async/promise support
	runner := jsrunner.NewEventLoopRunner()

	// Start the event loop in the background
	runner.Start()
	defer runner.Stop()

	// Await a promise that resolves after a timeout
	result, err := runner.AwaitPromise(`
		new Promise(function(resolve) {
			setTimeout(function() {
				resolve("Promise resolved!");
			}, 50);
		})
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)

	// Output:
	// Promise resolved!
}

func ExampleEventLoopRunner_asyncAwait() {
	runner := jsrunner.NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	// Use async/await syntax
	result, err := runner.AwaitPromise(`
		(async function() {
			// Simulate async operations
			await new Promise(resolve => setTimeout(resolve, 10));
			var x = 10;
			await new Promise(resolve => setTimeout(resolve, 10));
			var y = 20;
			return x + y;
		})()
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)

	// Output:
	// 30
}

func ExampleEventLoopRunner_promiseChain() {
	runner := jsrunner.NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	// Chain multiple promises
	result, err := runner.AwaitPromise(`
		Promise.resolve(5)
			.then(function(x) { return x * 2; })
			.then(function(x) { return x + 3; })
			.then(function(x) { return "Result: " + x; })
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)

	// Output:
	// Result: 13
}

func ExampleEventLoopRunner_goFunctions() {
	runner := jsrunner.NewEventLoopRunner()

	// Set Go functions that can be called from JavaScript
	runner.SetGlobal("processData", func(data string) string {
		return "Processed: " + data
	})

	runner.Start()
	defer runner.Stop()

	result, err := runner.AwaitPromise(`
		Promise.resolve("test data")
			.then(function(data) {
				return processData(data);
			})
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)

	// Output:
	// Processed: test data
}

func ExampleEventLoopRunner_setTimeout() {
	runner := jsrunner.NewEventLoopRunner()
	runner.Start()
	defer runner.Stop()

	// Use JavaScript's setTimeout
	result, err := runner.AwaitPromise(`
		new Promise(function(resolve) {
			var values = [];
			setTimeout(function() { values.push(1); }, 10);
			setTimeout(function() { values.push(2); }, 20);
			setTimeout(function() { 
				values.push(3);
				resolve(values.join(","));
			}, 30);
		})
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)

	// Output:
	// 1,2,3
}

func ExampleEventLoopRunner_runWithVM() {
	runner := jsrunner.NewEventLoopRunner()

	// Use Run for direct VM access
	runner.Run(func(vm *goja.Runtime) {
		vm.Set("counter", 0)
		vm.RunString(`
			function increment() {
				counter++;
				return counter;
			}
		`)
		result, _ := vm.RunString("increment()")
		fmt.Println(result.ToInteger())
	})

	// Output:
	// 1
}
