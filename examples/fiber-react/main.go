package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	jsrunner "github.com/boomhut/goja-runner"
	"github.com/gofiber/fiber/v2"
)

type ReactRenderer struct {
	app            *jsrunner.ReactApp
	bundleDuration time.Duration
}

var (
	_, currentFile, _, _ = runtime.Caller(0)
	exampleDir           = filepath.Dir(currentFile)
	assetsDir            = filepath.Join(exampleDir, "assets")
)

func NewReactRenderer() (*ReactRenderer, error) {
	bootStart := time.Now()

	polyfills, err := os.ReadFile(filepath.Join(assetsDir, "polyfills.js"))
	if err != nil {
		return nil, fmt.Errorf("failed to read polyfills: %w", err)
	}

	app, err := jsrunner.NewReactApp(jsrunner.ReactAppOptions{
		RunnerOptions: []jsrunner.Option{
			jsrunner.WithWebAccess(&jsrunner.WebAccessConfig{Timeout: 5 * time.Second}),
		},
		Polyfills:   []string{string(polyfills)},
		SSREntry:    defaultSSREntry,
		ClientEntry: defaultClientEntry,
	})
	if err != nil {
		return nil, err
	}

	bundleDuration := time.Since(bootStart)
	log.Printf("ReactApp bundled in %s", bundleDuration)

	return &ReactRenderer{app: app, bundleDuration: bundleDuration}, nil
}

func (rr *ReactRenderer) Render(props map[string]interface{}) (string, error) {
	return rr.app.Render(props)
}

func (rr *ReactRenderer) ClientBundle() string {
	return rr.app.ClientBundle()
}

func (rr *ReactRenderer) BundleDuration() time.Duration {
	return rr.bundleDuration
}

func main() {
	renderer, err := NewReactRenderer()
	if err != nil {
		log.Fatalf("boot renderer: %v", err)
	}
	log.Printf("React renderer ready (bundle %.2fms)", millis(renderer.BundleDuration()))

	app := fiber.New()

	app.Get("/", func(c *fiber.Ctx) error {
		reqStart := time.Now()
		props := map[string]interface{}{
			"user":      "Fiber",
			"timestamp": time.Now().Format(time.RFC3339),
			"message":   "Hello from goja-runner + React",
		}

		renderStart := time.Now()
		markup, err := renderer.Render(props)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		renderDuration := time.Since(renderStart)

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

		requestDuration := time.Since(reqStart)
		c.Set("Server-Timing", fmt.Sprintf("render;dur=%.2f,request;dur=%.2f", millis(renderDuration), millis(requestDuration)))
		log.Printf("GET / render=%s total=%s", renderDuration, requestDuration)

		return c.Type("html").SendString(html)
	})

	app.Get("/static/client.bundle.js", func(c *fiber.Ctx) error {
		c.Type("js")
		return c.SendString(renderer.ClientBundle())
	})

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

func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

const defaultSSREntry = `import React from "react";
import ReactDOMServer from "react-dom/server";

type HeroProps = {
	message?: string;
	timestamp?: string;
};

const Hero: React.FC<HeroProps> = (props) => (
	<section className="hero">
		<h1>goja-runner + Fiber</h1>
		<p>{props.message ?? "Server-side rendering from Go"}</p>
		<small>Rendered at {props.timestamp}</small>
	</section>
);

function stableStringify(value: Record<string, unknown>) {
	const sorted: Record<string, unknown> = {};
	Object.keys(value)
		.sort()
		.forEach((key) => {
			sorted[key] = value[key];
		});
	return JSON.stringify(sorted, null, 2);
}

const App: React.FC<Record<string, unknown>> = (props) => (
	<React.Fragment>
		<Hero
			message={(props as any).message as string}
			timestamp={(props as any).timestamp as string}
		/>
		<pre style={{ background: "#111", color: "#0f0", padding: "8px" }}>
			{stableStringify(props)}
		</pre>
	</React.Fragment>
);

export function renderApp(props: Record<string, unknown>) {
	return ReactDOMServer.renderToString(<App {...props} />);
}

if (typeof globalThis !== "undefined") {
	(globalThis as any).renderApp = renderApp;
}
`

const defaultClientEntry = `import React from "react";
import { hydrateRoot } from "react-dom/client";

type AppProps = Record<string, unknown>;

const Hero: React.FC<AppProps> = (props) => (
	<section className="hero">
		<h1>goja-runner + Fiber</h1>
		<p>{(props.message as string) ?? "Server-side rendering from Go"}</p>
		<small>Rendered at {props.timestamp as string}</small>
	</section>
);

function stableStringify(value: AppProps) {
	const sorted: AppProps = {};
	Object.keys(value)
		.sort()
		.forEach((key) => {
			sorted[key] = value[key];
		});
	return JSON.stringify(sorted, null, 2);
}

const App: React.FC<AppProps> = (props) => (
	<React.Fragment>
		<Hero {...props} />
		<pre style={{ background: "#111", color: "#0f0", padding: "8px" }}>
			{stableStringify(props)}
		</pre>
	</React.Fragment>
);

function boot() {
	const container = document.getElementById("root");
	if (!container) return;
	const props = (window as any).__INITIAL_PROPS__ ?? {};
	if (!props.timestamp) {
		props.timestamp = new Date().toISOString();
	}
	hydrateRoot(container, <App {...props} />);
}

if (typeof window !== "undefined") {
	window.addEventListener("DOMContentLoaded", boot);
}
`
