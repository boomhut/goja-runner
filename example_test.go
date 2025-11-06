package jsrunner_test

import (
	"fmt"
	"log"

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
