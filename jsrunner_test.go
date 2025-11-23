package jsrunner

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	runner := New()
	if runner == nil {
		t.Fatal("New() returned nil")
	}
	if runner.vm == nil {
		t.Error("vm is nil")
	}
	if runner.globals == nil {
		t.Error("globals map is nil")
	}
}

func TestSetGlobal(t *testing.T) {
	runner := New()

	// Test setting different types
	runner.SetGlobal("stringVar", "test")
	runner.SetGlobal("intVar", 42)
	runner.SetGlobal("boolVar", true)
	runner.SetGlobal("floatVar", 3.14)

	// Verify we can access them in JS
	result, err := runner.Eval("stringVar")
	if err != nil {
		t.Fatalf("Failed to eval stringVar: %v", err)
	}
	if ExportString(result) != "test" {
		t.Errorf("Expected 'test', got '%s'", ExportString(result))
	}

	result, err = runner.Eval("intVar")
	if err != nil {
		t.Fatalf("Failed to eval intVar: %v", err)
	}
	if ExportInt(result) != 42 {
		t.Errorf("Expected 42, got %d", ExportInt(result))
	}

	result, err = runner.Eval("boolVar")
	if err != nil {
		t.Fatalf("Failed to eval boolVar: %v", err)
	}
	if !ExportBool(result) {
		t.Error("Expected true, got false")
	}

	result, err = runner.Eval("floatVar")
	if err != nil {
		t.Fatalf("Failed to eval floatVar: %v", err)
	}
	if ExportFloat(result) != 3.14 {
		t.Errorf("Expected 3.14, got %f", ExportFloat(result))
	}
}

func TestLoadScriptString(t *testing.T) {
	runner := New()

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "valid code",
			code:    "var x = 5;",
			wantErr: false,
		},
		{
			name:    "function declaration",
			code:    "function test() { return 42; }",
			wantErr: false,
		},
		{
			name:    "invalid syntax",
			code:    "var x = ;",
			wantErr: true,
		},
		{
			name:    "empty code",
			code:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runner.LoadScriptString(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadScriptString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadScript(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.js")
	err := os.WriteFile(validFile, []byte("var testVar = 'loaded';"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	invalidFile := filepath.Join(tmpDir, "invalid.js")
	err = os.WriteFile(invalidFile, []byte("var x = ;"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{
			name:    "valid file",
			file:    validFile,
			wantErr: false,
		},
		{
			name:    "invalid syntax",
			file:    invalidFile,
			wantErr: true,
		},
		{
			name:    "non-existent file",
			file:    filepath.Join(tmpDir, "nonexistent.js"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := New()
			err := runner.LoadScript(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadScript() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadScriptAndAccess(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.js")
	err := os.WriteFile(testFile, []byte("var loaded = 'success';"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	runner := New()
	err = runner.LoadScript(testFile)
	if err != nil {
		t.Fatalf("LoadScript() failed: %v", err)
	}

	result, err := runner.Eval("loaded")
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if ExportString(result) != "success" {
		t.Errorf("Expected 'success', got '%s'", ExportString(result))
	}
}

func TestCall(t *testing.T) {
	runner := New()
	code := `
		function add(a, b) {
			return a + b;
		}
		function greet(name) {
			return "Hello, " + name;
		}
		function multiply(a, b, c) {
			return a * b * c;
		}
		function returnBool() {
			return true;
		}
	`
	err := runner.LoadScriptString(code)
	if err != nil {
		t.Fatalf("LoadScriptString() failed: %v", err)
	}

	tests := []struct {
		name     string
		function string
		args     []interface{}
		want     interface{}
		wantErr  bool
	}{
		{
			name:     "add integers",
			function: "add",
			args:     []interface{}{5, 3},
			want:     int64(8),
			wantErr:  false,
		},
		{
			name:     "greet with string",
			function: "greet",
			args:     []interface{}{"World"},
			want:     "Hello, World",
			wantErr:  false,
		},
		{
			name:     "multiply three numbers",
			function: "multiply",
			args:     []interface{}{2, 3, 4},
			want:     int64(24),
			wantErr:  false,
		},
		{
			name:     "no arguments",
			function: "returnBool",
			args:     []interface{}{},
			want:     true,
			wantErr:  false,
		},
		{
			name:     "non-existent function",
			function: "nonExistent",
			args:     []interface{}{},
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Call(tt.function, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Call() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				switch v := tt.want.(type) {
				case int64:
					if ExportInt(result) != v {
						t.Errorf("Expected %d, got %d", v, ExportInt(result))
					}
				case string:
					if ExportString(result) != v {
						t.Errorf("Expected '%s', got '%s'", v, ExportString(result))
					}
				case bool:
					if ExportBool(result) != v {
						t.Errorf("Expected %v, got %v", v, ExportBool(result))
					}
				}
			}
		})
	}
}

func TestCallWithDifferentTypes(t *testing.T) {
	runner := New()
	code := `
		function processArgs(str, num, flt, bool) {
			return typeof str + "," + typeof num + "," + typeof flt + "," + typeof bool;
		}
	`
	err := runner.LoadScriptString(code)
	if err != nil {
		t.Fatalf("LoadScriptString() failed: %v", err)
	}

	result, err := runner.Call("processArgs", "test", 42, 3.14, true)
	if err != nil {
		t.Fatalf("Call() failed: %v", err)
	}

	expected := "string,number,number,boolean"
	if ExportString(result) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, ExportString(result))
	}
}

func TestEval(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		want       interface{}
		wantErr    bool
	}{
		{
			name:       "simple math",
			expression: "2 + 2",
			want:       int64(4),
			wantErr:    false,
		},
		{
			name:       "string concatenation",
			expression: "'Hello' + ' ' + 'World'",
			want:       "Hello World",
			wantErr:    false,
		},
		{
			name:       "array operation",
			expression: "[1, 2, 3].length",
			want:       int64(3),
			wantErr:    false,
		},
		{
			name:       "boolean expression",
			expression: "5 > 3",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "invalid syntax",
			expression: "2 +",
			want:       nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("Eval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				switch v := tt.want.(type) {
				case int64:
					if ExportInt(result) != v {
						t.Errorf("Expected %d, got %d", v, ExportInt(result))
					}
				case string:
					if ExportString(result) != v {
						t.Errorf("Expected '%s', got '%s'", v, ExportString(result))
					}
				case bool:
					if ExportBool(result) != v {
						t.Errorf("Expected %v, got %v", v, ExportBool(result))
					}
				}
			}
		})
	}
}

func TestGetVM(t *testing.T) {
	runner := New()
	vm := runner.GetVM()
	if vm == nil {
		t.Error("GetVM() returned nil")
	}
	if vm != runner.vm {
		t.Error("GetVM() returned different VM instance")
	}
}

func TestExportString(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{
			name:       "string value",
			expression: "'test'",
			want:       "test",
		},
		{
			name:       "number to string",
			expression: "42",
			want:       "42",
		},
		{
			name:       "boolean to string",
			expression: "true",
			want:       "true",
		},
		{
			name:       "null",
			expression: "null",
			want:       "null",
		},
		{
			name:       "undefined",
			expression: "undefined",
			want:       "undefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if err != nil {
				t.Fatalf("Eval() failed: %v", err)
			}
			got := ExportString(result)
			if got != tt.want {
				t.Errorf("ExportString() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test nil value
	if ExportString(nil) != "" {
		t.Error("ExportString(nil) should return empty string")
	}
}

func TestExportInt(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		want       int64
	}{
		{
			name:       "positive integer",
			expression: "42",
			want:       42,
		},
		{
			name:       "negative integer",
			expression: "-10",
			want:       -10,
		},
		{
			name:       "zero",
			expression: "0",
			want:       0,
		},
		{
			name:       "float to int",
			expression: "3.14",
			want:       3,
		},
		{
			name:       "string to int",
			expression: "'123'",
			want:       123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if err != nil {
				t.Fatalf("Eval() failed: %v", err)
			}
			got := ExportInt(result)
			if got != tt.want {
				t.Errorf("ExportInt() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test nil value
	if ExportInt(nil) != 0 {
		t.Error("ExportInt(nil) should return 0")
	}
}

func TestExportFloat(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		want       float64
	}{
		{
			name:       "float value",
			expression: "3.14",
			want:       3.14,
		},
		{
			name:       "integer to float",
			expression: "42",
			want:       42.0,
		},
		{
			name:       "negative float",
			expression: "-2.5",
			want:       -2.5,
		},
		{
			name:       "zero",
			expression: "0.0",
			want:       0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if err != nil {
				t.Fatalf("Eval() failed: %v", err)
			}
			got := ExportFloat(result)
			if got != tt.want {
				t.Errorf("ExportFloat() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test nil value
	if ExportFloat(nil) != 0 {
		t.Error("ExportFloat(nil) should return 0")
	}
}

func TestExportBool(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{
			name:       "true",
			expression: "true",
			want:       true,
		},
		{
			name:       "false",
			expression: "false",
			want:       false,
		},
		{
			name:       "truthy value",
			expression: "1",
			want:       true,
		},
		{
			name:       "falsy value",
			expression: "0",
			want:       false,
		},
		{
			name:       "truthy string",
			expression: "'hello'",
			want:       true,
		},
		{
			name:       "empty string",
			expression: "''",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if err != nil {
				t.Fatalf("Eval() failed: %v", err)
			}
			got := ExportBool(result)
			if got != tt.want {
				t.Errorf("ExportBool() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test nil value
	if ExportBool(nil) != false {
		t.Error("ExportBool(nil) should return false")
	}
}

func TestExport(t *testing.T) {
	runner := New()

	tests := []struct {
		name       string
		expression string
		checkType  string
	}{
		{
			name:       "string",
			expression: "'test'",
			checkType:  "string",
		},
		{
			name:       "number",
			expression: "42",
			checkType:  "float64",
		},
		{
			name:       "boolean",
			expression: "true",
			checkType:  "bool",
		},
		{
			name:       "object",
			expression: "{key: 'value'}",
			checkType:  "map",
		},
		{
			name:       "array",
			expression: "[1, 2, 3]",
			checkType:  "slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.Eval(tt.expression)
			if err != nil {
				t.Fatalf("Eval() failed: %v", err)
			}
			exported := Export(result)
			if exported == nil {
				t.Error("Export() returned nil")
			}
		})
	}

	// Test nil value
	if Export(nil) != nil {
		t.Error("Export(nil) should return nil")
	}
}

func TestComplexScenario(t *testing.T) {
	runner := New()

	// Set up globals
	runner.SetGlobal("multiplier", 2)

	// Load a script
	code := `
		function process(input) {
			return input * multiplier;
		}
		
		var state = {
			count: 0
		};
		
		function increment() {
			state.count++;
			return state.count;
		}
	`
	err := runner.LoadScriptString(code)
	if err != nil {
		t.Fatalf("LoadScriptString() failed: %v", err)
	}

	// Call function with global
	result, err := runner.Call("process", 5)
	if err != nil {
		t.Fatalf("Call() failed: %v", err)
	}
	if ExportInt(result) != 10 {
		t.Errorf("Expected 10, got %d", ExportInt(result))
	}

	// Call stateful function multiple times
	for i := 1; i <= 3; i++ {
		result, err := runner.Call("increment")
		if err != nil {
			t.Fatalf("Call() failed: %v", err)
		}
		if ExportInt(result) != int64(i) {
			t.Errorf("Expected %d, got %d", i, ExportInt(result))
		}
	}

	// Verify state persists
	result, err = runner.Eval("state.count")
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if ExportInt(result) != 3 {
		t.Errorf("Expected count to be 3, got %d", ExportInt(result))
	}
}

func TestNewWithGlobals(t *testing.T) {
	globals := map[string]interface{}{
		"apiKey":  "secret-123",
		"timeout": 30,
		"debug":   true,
	}

	runner := NewWithGlobals(globals)
	if runner == nil {
		t.Fatal("NewWithGlobals() returned nil")
	}

	// Verify globals are set
	result, err := runner.Eval("apiKey")
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if ExportString(result) != "secret-123" {
		t.Errorf("Expected 'secret-123', got '%s'", ExportString(result))
	}

	result, err = runner.Eval("timeout")
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if ExportInt(result) != 30 {
		t.Errorf("Expected 30, got %d", ExportInt(result))
	}

	result, err = runner.Eval("debug")
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if !ExportBool(result) {
		t.Error("Expected true, got false")
	}
}

func TestNewWithGlobalsEmptyMap(t *testing.T) {
	runner := NewWithGlobals(map[string]interface{}{})
	if runner == nil {
		t.Fatal("NewWithGlobals() returned nil")
	}
	if runner.vm == nil {
		t.Error("vm is nil")
	}
}

func TestSharedStatePointer(t *testing.T) {
	type SharedState struct {
		Counter int
		Data    string
	}

	state := &SharedState{
		Counter: 0,
		Data:    "initial",
	}

	globals := map[string]interface{}{
		"state": state,
	}

	// Create two runners with shared state
	runner1 := NewWithGlobals(globals)
	runner2 := NewWithGlobals(globals)

	// Modify state through Go
	state.Counter = 42
	state.Data = "modified"

	// Both runners should see the updated state
	result, err := runner1.Eval("state.Counter")
	if err != nil {
		t.Fatalf("runner1 Eval() failed: %v", err)
	}
	if ExportInt(result) != 42 {
		t.Errorf("runner1: Expected 42, got %d", ExportInt(result))
	}

	result, err = runner2.Eval("state.Counter")
	if err != nil {
		t.Fatalf("runner2 Eval() failed: %v", err)
	}
	if ExportInt(result) != 42 {
		t.Errorf("runner2: Expected 42, got %d", ExportInt(result))
	}

	result, err = runner1.Eval("state.Data")
	if err != nil {
		t.Fatalf("runner1 Eval() failed: %v", err)
	}
	if ExportString(result) != "modified" {
		t.Errorf("runner1: Expected 'modified', got '%s'", ExportString(result))
	}

	result, err = runner2.Eval("state.Data")
	if err != nil {
		t.Fatalf("runner2 Eval() failed: %v", err)
	}
	if ExportString(result) != "modified" {
		t.Errorf("runner2: Expected 'modified', got '%s'", ExportString(result))
	}
}

func TestConcurrentRunners(t *testing.T) {
	type SharedState struct {
		Value int
	}

	state := &SharedState{Value: 100}
	globals := map[string]interface{}{
		"state": state,
	}

	// Test that multiple runners can be used concurrently
	// Each runner is only accessed by one goroutine
	done := make(chan bool, 2)

	go func() {
		runner := NewWithGlobals(globals)
		err := runner.LoadScriptString(`
			function getValue() {
				return state.Value;
			}
		`)
		if err != nil {
			t.Errorf("goroutine 1: LoadScriptString() failed: %v", err)
		}

		for i := 0; i < 100; i++ {
			result, err := runner.Call("getValue")
			if err != nil {
				t.Errorf("goroutine 1: Call() failed: %v", err)
				break
			}
			if ExportInt(result) != 100 {
				t.Errorf("goroutine 1: Expected 100, got %d", ExportInt(result))
				break
			}
		}
		done <- true
	}()

	go func() {
		runner := NewWithGlobals(globals)
		err := runner.LoadScriptString(`
			function calculate() {
				return state.Value * 2;
			}
		`)
		if err != nil {
			t.Errorf("goroutine 2: LoadScriptString() failed: %v", err)
		}

		for i := 0; i < 100; i++ {
			result, err := runner.Call("calculate")
			if err != nil {
				t.Errorf("goroutine 2: Call() failed: %v", err)
				break
			}
			if ExportInt(result) != 200 {
				t.Errorf("goroutine 2: Expected 200, got %d", ExportInt(result))
				break
			}
		}
		done <- true
	}()

	<-done
	<-done
}

func TestConcurrentRunnersIndependentScripts(t *testing.T) {
	// Test that runners in different goroutines maintain independent JS environments
	done := make(chan bool, 2)

	go func() {
		runner := New()
		err := runner.LoadScriptString(`var x = 1;`)
		if err != nil {
			t.Errorf("goroutine 1: LoadScriptString() failed: %v", err)
		}

		for i := 0; i < 50; i++ {
			runner.LoadScriptString(`x = x + 1;`)
		}

		result, err := runner.Eval("x")
		if err != nil {
			t.Errorf("goroutine 1: Eval() failed: %v", err)
		}
		if ExportInt(result) != 51 {
			t.Errorf("goroutine 1: Expected 51, got %d", ExportInt(result))
		}
		done <- true
	}()

	go func() {
		runner := New()
		err := runner.LoadScriptString(`var x = 100;`)
		if err != nil {
			t.Errorf("goroutine 2: LoadScriptString() failed: %v", err)
		}

		for i := 0; i < 50; i++ {
			runner.LoadScriptString(`x = x - 1;`)
		}

		result, err := runner.Eval("x")
		if err != nil {
			t.Errorf("goroutine 2: Eval() failed: %v", err)
		}
		if ExportInt(result) != 50 {
			t.Errorf("goroutine 2: Expected 50, got %d", ExportInt(result))
		}
		done <- true
	}()

	<-done
	<-done
}

func TestSharedGlobalsIsolatedJSEnvironments(t *testing.T) {
	// Test that while Go state is shared, JS environments remain isolated
	globals := map[string]interface{}{
		"apiKey": "shared-key",
	}

	runner1 := NewWithGlobals(globals)
	runner2 := NewWithGlobals(globals)

	// Define a JS variable in runner1
	err := runner1.LoadScriptString(`var localVar = "runner1";`)
	if err != nil {
		t.Fatalf("runner1 LoadScriptString() failed: %v", err)
	}

	// Define a JS variable in runner2
	err = runner2.LoadScriptString(`var localVar = "runner2";`)
	if err != nil {
		t.Fatalf("runner2 LoadScriptString() failed: %v", err)
	}

	// Verify both can access shared global
	result, err := runner1.Eval("apiKey")
	if err != nil {
		t.Fatalf("runner1 Eval(apiKey) failed: %v", err)
	}
	if ExportString(result) != "shared-key" {
		t.Errorf("runner1: Expected 'shared-key', got '%s'", ExportString(result))
	}

	result, err = runner2.Eval("apiKey")
	if err != nil {
		t.Fatalf("runner2 Eval(apiKey) failed: %v", err)
	}
	if ExportString(result) != "shared-key" {
		t.Errorf("runner2: Expected 'shared-key', got '%s'", ExportString(result))
	}

	// Verify JS environments are isolated
	result, err = runner1.Eval("localVar")
	if err != nil {
		t.Fatalf("runner1 Eval(localVar) failed: %v", err)
	}
	if ExportString(result) != "runner1" {
		t.Errorf("runner1: Expected 'runner1', got '%s'", ExportString(result))
	}

	result, err = runner2.Eval("localVar")
	if err != nil {
		t.Fatalf("runner2 Eval(localVar) failed: %v", err)
	}
	if ExportString(result) != "runner2" {
		t.Errorf("runner2: Expected 'runner2', got '%s'", ExportString(result))
	}
}

func TestConcurrentMutationsWithSync(t *testing.T) {
	// Test that shared mutable state works correctly across goroutines with proper synchronization
	type Counter struct {
		mu    sync.Mutex
		value int
	}

	counter := &Counter{value: 0}

	// Create methods that JS can call
	increment := func() int {
		counter.mu.Lock()
		defer counter.mu.Unlock()
		counter.value++
		return counter.value
	}

	getValue := func() int {
		counter.mu.Lock()
		defer counter.mu.Unlock()
		return counter.value
	}

	globals := map[string]interface{}{
		"increment": increment,
		"getValue":  getValue,
	}

	const numGoroutines = 10
	const incrementsPerGoroutine = 100
	done := make(chan bool, numGoroutines)

	// Launch multiple goroutines that increment the counter
	for i := 0; i < numGoroutines; i++ {
		go func() {
			runner := NewWithGlobals(globals)
			for j := 0; j < incrementsPerGoroutine; j++ {
				_, err := runner.Eval("increment()")
				if err != nil {
					t.Errorf("increment() failed: %v", err)
					break
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify the final count
	runner := NewWithGlobals(globals)
	result, err := runner.Eval("getValue()")
	if err != nil {
		t.Fatalf("getValue() failed: %v", err)
	}

	expected := int64(numGoroutines * incrementsPerGoroutine)
	actual := ExportInt(result)
	if actual != expected {
		t.Errorf("Expected counter to be %d, got %d", expected, actual)
	}
}
