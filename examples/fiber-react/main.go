package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	jsrunner "github.com/boomhut/goja-runner"
	"github.com/gofiber/fiber/v2"
)

type ReactRenderer struct {
	app            *jsrunner.ReactApp
	bundleDuration time.Duration
	metrics        *perfMetrics
}

type perfMetrics struct {
	mu              sync.Mutex
	totalRequests   int
	totalRender     time.Duration
	totalRequest    time.Duration
	lastRender      time.Duration
	lastRequest     time.Duration
	metricsFetches  int
	lastFetchRender time.Duration
	lastFetchTotal  time.Duration
}

func newPerfMetrics() *perfMetrics {
	return &perfMetrics{}
}

func (pm *perfMetrics) Record(renderDur, requestDur time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.totalRequests++
	pm.lastRender = renderDur
	pm.lastRequest = requestDur
	pm.totalRender += renderDur
	pm.totalRequest += requestDur
}

func (pm *perfMetrics) RecordFetch(renderDur, totalDur time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.metricsFetches++
	pm.lastFetchRender = renderDur
	pm.lastFetchTotal = totalDur
}

func (pm *perfMetrics) Snapshot(bundle time.Duration) map[string]interface{} {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	avgRender := 0.0
	avgRequest := 0.0
	if pm.totalRequests > 0 {
		avgRender = millis(pm.totalRender) / float64(pm.totalRequests)
		avgRequest = millis(pm.totalRequest) / float64(pm.totalRequests)
	}

	lastRenderMs := millis(pm.lastRender)
	lastRequestMs := millis(pm.lastRequest)
	if pm.metricsFetches > 0 {
		lastRenderMs = millis(pm.lastFetchRender)
		lastRequestMs = millis(pm.lastFetchTotal)
	}

	return map[string]interface{}{
		"totalRequests": pm.totalRequests + pm.metricsFetches,
		"lastRenderMs":  lastRenderMs,
		"lastRequestMs": lastRequestMs,
		"avgRenderMs":   avgRender,
		"avgRequestMs":  avgRequest,
		"bundleMs":      millis(bundle),
		"generatedAt":   time.Now().Format(time.RFC3339Nano),
	}
}

var (
	_, currentFile, _, _ = runtime.Caller(0)
	exampleDir           = filepath.Dir(currentFile)
	assetsDir            = filepath.Join(exampleDir, "assets")
)

func NewReactRenderer() (*ReactRenderer, error) {
	bootStart := time.Now()
	metrics := newPerfMetrics()

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

	return &ReactRenderer{app: app, bundleDuration: bundleDuration, metrics: metrics}, nil
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

func (rr *ReactRenderer) MetricsSnapshot() map[string]interface{} {
	if rr.metrics == nil {
		return nil
	}
	return rr.metrics.Snapshot(rr.BundleDuration())
}

func (rr *ReactRenderer) RecordMetrics(renderDur, requestDur time.Duration) {
	if rr.metrics == nil {
		return
	}
	rr.metrics.Record(renderDur, requestDur)
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
			"bundleMs":  millis(renderer.BundleDuration()),
			"metrics":   renderer.MetricsSnapshot(),
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
    <style>%s</style>
  </head>
  <body>
    <div id="root">%s</div>
    <script>
      window.__INITIAL_PROPS__ = %s;
    </script>
    <script src="/static/client.bundle.js"></script>
  </body>
</html>`, pageStyles, markup, mustJSON(props))

		requestDuration := time.Since(reqStart)
		renderer.RecordMetrics(renderDuration, requestDuration)
		c.Set("Server-Timing", fmt.Sprintf("render;dur=%.2f,request;dur=%.2f", millis(renderDuration), millis(requestDuration)))
		log.Printf("GET / render=%s total=%s", renderDuration, requestDuration)

		return c.Type("html").SendString(html)
	})

	app.Get("/static/client.bundle.js", func(c *fiber.Ctx) error {
		c.Type("js")
		return c.SendString(renderer.ClientBundle())
	})

	app.Get("/metrics", func(c *fiber.Ctx) error {
		reqStart := time.Now()

		// Simulate a small render operation
		renderStart := time.Now()
		snapshot := renderer.MetricsSnapshot()
		renderDuration := time.Since(renderStart)

		// Record this metrics fetch
		totalDuration := time.Since(reqStart)
		renderer.metrics.RecordFetch(renderDuration, totalDuration)

		c.Set("Cache-Control", "no-store, max-age=0")
		return c.JSON(fiber.Map{
			"metrics": snapshot,
		})
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

const pageStyles = `:root {
	color-scheme: dark;
	font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

body {
	margin: 0;
	min-height: 100vh;
	background: #0d1117;
	color: #e6edf3;
}

#root {
	max-width: 960px;
	margin: 0 auto;
	padding: 24px;
}

.hero {
	background: radial-gradient(circle at top left, #1f6feb, #0d1117 70%);
	padding: 24px;
	border-radius: 16px;
	box-shadow: 0 20px 50px rgba(0, 0, 0, 0.35);
}

.hero h1 {
	margin: 0 0 8px;
	font-size: 2rem;
}

.hero p {
	margin: 0;
	font-size: 1.125rem;
}

.metrics {
	margin-top: 32px;
	padding: 20px;
	border: 1px solid rgba(255, 255, 255, 0.1);
	border-radius: 16px;
	background: rgba(13, 17, 23, 0.8);
	backdrop-filter: blur(8px);
}

.metrics ul {
	list-style: none;
	padding: 0;
	margin: 16px 0;
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	gap: 12px;
}

.metric-row {
	display: flex;
	flex-direction: column;
	padding: 12px;
	border-radius: 12px;
	background: rgba(255, 255, 255, 0.04);
}

.metric-label {
	font-size: 0.85rem;
	text-transform: uppercase;
	letter-spacing: 0.08em;
	color: rgba(230, 237, 243, 0.7);
}

.metric-value {
	font-size: 1.35rem;
	font-weight: 600;
}

.metrics-actions {
	display: flex;
	align-items: center;
	gap: 12px;
	margin-top: 8px;
}

.metrics-actions button {
	border: none;
	border-radius: 999px;
	padding: 10px 18px;
	font-weight: 600;
	background: #1f6feb;
	color: white;
	cursor: pointer;
}

.metrics-actions button[disabled] {
	opacity: 0.6;
	cursor: wait;
}

.metrics-actions .error {
	color: #ffa198;
}

.props-dump {
	margin-top: 32px;
	font-size: 0.9rem;
	background: #161b22;
	padding: 16px;
	border-radius: 12px;
	overflow-x: auto;
}`

const defaultSSREntry = `import React from "react";
import ReactDOMServer from "react-dom/server";

type MetricsSnapshot = {
	totalRequests?: number;
	lastRenderMs?: number;
	lastRequestMs?: number;
	avgRenderMs?: number;
	avgRequestMs?: number;
	bundleMs?: number;
	generatedAt?: string;
};

type AppProps = {
	message?: string;
	timestamp?: string;
	bundleMs?: number;
	metrics?: MetricsSnapshot;
};

const Hero: React.FC<AppProps> = (props) => (
	<section className="hero">
		<h1>goja-runner + Fiber</h1>
		<p>{props.message ?? "Server-side rendering from Go"}</p>
		<small>Rendered at {props.timestamp}</small>
	</section>
);

const MetricRow: React.FC<{ label: string; value: string }> = ({ label, value }) => (
	<li className="metric-row">
		<span className="metric-label">{label}</span>
		<span className="metric-value">{value}</span>
	</li>
);

const MetricsPanel: React.FC<{ metrics?: MetricsSnapshot }> = ({ metrics }) => {
	if (!metrics) {
		return (
			<section className="metrics">
				<h2>Runtime Metrics</h2>
				<p>No metrics yet. Refresh after a few requests.</p>
			</section>
		);
	}

	const summary = [
		{ label: "Bundle (ms)", value: Number(metrics.bundleMs ?? 0).toFixed(2) },
		{ label: "Last render (ms)", value: Number(metrics.lastRenderMs ?? 0).toFixed(2) },
		{ label: "Last request (ms)", value: Number(metrics.lastRequestMs ?? 0).toFixed(2) },
		{ label: "Avg render (ms)", value: Number(metrics.avgRenderMs ?? 0).toFixed(2) },
		{ label: "Avg request (ms)", value: Number(metrics.avgRequestMs ?? 0).toFixed(2) },
		{ label: "Total requests", value: String(metrics.totalRequests ?? 0) },
	];

	return (
		<section className="metrics">
			<h2>Runtime Metrics</h2>
			<ul>
				{summary.map((entry) => (
					<MetricRow key={entry.label} label={entry.label} value={entry.value} />
				))}
			</ul>
			{metrics.generatedAt && <small>Generated at {metrics.generatedAt}</small>}
		</section>
	);
};

const MetricsConsoleShell: React.FC<{ metrics?: MetricsSnapshot }> = ({ metrics }) => (
	<div>
		<MetricsPanel metrics={metrics} />
		<div className="metrics-actions">
			<button>Refresh metrics</button>
		</div>
	</div>
);

function deepSort(value: any): any {
	if (value === null || value === undefined || typeof value !== "object") {
		return value;
	}
	if (Array.isArray(value)) {
		return value.map(deepSort);
	}
	const sorted: Record<string, any> = {};
	Object.keys(value)
		.sort()
		.forEach((key) => {
			sorted[key] = deepSort(value[key]);
		});
	return sorted;
}

function stableStringify(value: any): string {
	return JSON.stringify(deepSort(value), null, 2);
}

const App: React.FC<AppProps> = (props) => (
	<React.Fragment>
		<Hero {...props} />
		<MetricsConsoleShell metrics={props.metrics} />
		<pre className="props-dump">{stableStringify(props as Record<string, unknown>)}</pre>
	</React.Fragment>
);

export function renderApp(props: Record<string, unknown>) {
	return ReactDOMServer.renderToString(<App {...(props as AppProps)} />);
}

if (typeof globalThis !== "undefined") {
	(globalThis as any).renderApp = renderApp;
}
`

const defaultClientEntry = `import React from "react";
import { hydrateRoot } from "react-dom/client";

type MetricsSnapshot = {
	totalRequests?: number;
	lastRenderMs?: number;
	lastRequestMs?: number;
	avgRenderMs?: number;
	avgRequestMs?: number;
	bundleMs?: number;
	generatedAt?: string;
};

type AppProps = {
	message?: string;
	timestamp?: string;
	bundleMs?: number;
	metrics?: MetricsSnapshot;
};

const Hero: React.FC<AppProps> = (props) => (
	<section className="hero">
		<h1>goja-runner + Fiber</h1>
		<p>{props.message ?? "Server-side rendering from Go"}</p>
		<small>Rendered at {props.timestamp}</small>
	</section>
);

const MetricRow: React.FC<{ label: string; value: string }> = ({ label, value }) => (
	<li className="metric-row">
		<span className="metric-label">{label}</span>
		<span className="metric-value">{value}</span>
	</li>
);

const MetricsPanel: React.FC<{ metrics?: MetricsSnapshot }> = ({ metrics }) => {
	if (!metrics) {
		return (
			<section className="metrics">
				<h2>Runtime Metrics</h2>
				<p>No metrics yet. Refresh after a few requests.</p>
			</section>
		);
	}

	const summary = [
		{ label: "Bundle (ms)", value: Number(metrics.bundleMs ?? 0).toFixed(2) },
		{ label: "Last render (ms)", value: Number(metrics.lastRenderMs ?? 0).toFixed(2) },
		{ label: "Last request (ms)", value: Number(metrics.lastRequestMs ?? 0).toFixed(2) },
		{ label: "Avg render (ms)", value: Number(metrics.avgRenderMs ?? 0).toFixed(2) },
		{ label: "Avg request (ms)", value: Number(metrics.avgRequestMs ?? 0).toFixed(2) },
		{ label: "Total requests", value: String(metrics.totalRequests ?? 0) },
	];

	return (
		<section className="metrics">
			<h2>Runtime Metrics</h2>
			<ul>
				{summary.map((entry) => (
					<MetricRow key={entry.label} label={entry.label} value={entry.value} />
				))}
			</ul>
			{metrics.generatedAt && <small>Generated at {metrics.generatedAt}</small>}
		</section>
	);
};

const MetricsConsole: React.FC<{ metrics?: MetricsSnapshot }> = ({ metrics }) => {
	const [current, setCurrent] = React.useState<MetricsSnapshot | undefined>(metrics);
	const [pending, setPending] = React.useState(false);
	const [error, setError] = React.useState<string | null>(null);
	const [updateKey, setUpdateKey] = React.useState(0);

	const refresh = React.useCallback(async () => {
		if (typeof window === "undefined") {
			return;
		}
		setPending(true);
		setError(null);
		try {
			const response = await fetch("/metrics?ts=" + Date.now(), { 
				cache: "no-store",
				headers: { "Cache-Control": "no-cache" }
			});
			if (!response.ok) {
				throw new Error("status " + response.status);
			}
			const payload = (await response.json()) as { metrics?: MetricsSnapshot };
			setCurrent(payload.metrics);
			setUpdateKey(prev => prev + 1);
		} catch (err) {
			setError((err as Error).message);
		} finally {
			setPending(false);
		}
	}, []);

	return (
		<div>
			<MetricsPanel key={updateKey} metrics={current} />
			<div className="metrics-actions">
				<button onClick={refresh} disabled={pending}>
					{pending ? "Refreshing..." : "Refresh metrics"}
				</button>
				{error && <small className="error">Failed: {error}</small>}
			</div>
		</div>
	);
};

function deepSort(value: any): any {
	if (value === null || value === undefined || typeof value !== "object") {
		return value;
	}
	if (Array.isArray(value)) {
		return value.map(deepSort);
	}
	const sorted: Record<string, any> = {};
	Object.keys(value)
		.sort()
		.forEach((key) => {
			sorted[key] = deepSort(value[key]);
		});
	return sorted;
}

function stableStringify(value: any): string {
	return JSON.stringify(deepSort(value), null, 2);
}

const App: React.FC<AppProps> = (props) => (
	<React.Fragment>
		<Hero {...props} />
		<MetricsConsole metrics={props.metrics} />
		<pre className="props-dump">{stableStringify(props as Record<string, unknown>)}</pre>
	</React.Fragment>
);

function boot() {
	const container = document.getElementById("root");
	if (!container) return;
	const props = ((window as any).__INITIAL_PROPS__ ?? {}) as AppProps;
	if (!props.timestamp) {
		props.timestamp = new Date().toISOString();
	}
	hydrateRoot(container, <App {...props} />);
}

if (typeof window !== "undefined") {
	window.addEventListener("DOMContentLoaded", boot);
}
`
