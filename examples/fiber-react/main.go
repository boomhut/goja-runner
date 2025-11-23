package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	jsrunner "github.com/boomhut/goja-runner"
	"github.com/gofiber/fiber/v2"
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
			return nil, fmt.Errorf("failed to load %s bundle: %w", loader.name, err)
		}
	}

	return &ReactRenderer{runner: r}, nil
}

func (rr *ReactRenderer) Render(props map[string]interface{}) (string, error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	rr.runner.SetGlobal("SERVER_PROPS", props)

	markup, err := rr.runner.Eval("renderApp(SERVER_PROPS)")
	if err != nil {
		return "", fmt.Errorf("renderApp failed: %w", err)
	}

	return jsrunner.ExportString(markup), nil
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
    <title>goja-runner + React SSR</title>
  </head>
  <body>
    <div id="root">%s</div>
    <script>
      window.__INITIAL_PROPS__ = %s;
    </script>
    <script src="/static/client.bundle.js"></script>
  </body>
</html>`, markup, mustJSON(props))

		return c.Type("html").SendString(html)
	})

	app.Static("/static", "./assets/public")

	log.Println("listening on http://localhost:3000")
	log.Fatal(app.Listen(":3000"))
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
