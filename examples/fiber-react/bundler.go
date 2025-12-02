package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

type reactBundles struct {
	SSR    string
	Client string
}

func buildReactBundles() (*reactBundles, error) {
	resolver := newRemoteResolver()

	ssr, err := buildBundle(ssrEntryPoint, "app-ssr.tsx", api.PlatformNode, resolver)
	if err != nil {
		return nil, fmt.Errorf("bundle ssr: %w", err)
	}

	client, err := buildBundle(clientEntryPoint, "app-client.tsx", api.PlatformBrowser, resolver)
	if err != nil {
		return nil, fmt.Errorf("bundle client: %w", err)
	}

	return &reactBundles{SSR: ssr, Client: client}, nil
}

func buildBundle(entry, sourceFile string, platform api.Platform, resolver *remoteResolver) (string, error) {
	result := api.Build(api.BuildOptions{
		Bundle:           true,
		Format:           api.FormatIIFE,
		Platform:         platform,
		Target:           api.ES2018,
		MinifyWhitespace: true,
		Write:            false,
		JSX:              api.JSXAutomatic,
		Define: map[string]string{
			"process.env.NODE_ENV": "\"development\"",
		},
		Plugins: []api.Plugin{resolver.Plugin()},
		Stdin: &api.StdinOptions{
			Contents:   entry,
			Loader:     api.LoaderTSX,
			ResolveDir: ".",
			Sourcefile: sourceFile,
		},
	})

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("esbuild error: %s", result.Errors[0].Text)
	}
	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild produced no output")
	}
	return string(result.OutputFiles[0].Contents), nil
}

const reactVersion = "18.3.1"

var ssrEntryPoint = `import React from "react";
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
        <Hero message={(props as any).message as string} timestamp={(props as any).timestamp as string} />
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

var clientEntryPoint = `import React from "react";
import { hydrateRoot } from "react-dom/client";

type AppProps = Record<string, unknown>;

const Hero: React.FC<AppProps> = (props) => (
	<section className="hero">
		<h1>goja-runner + Fiber</h1>
		<p>{(props.message as string) ?? "Server-side rendering from Go"}</p>
		<small>Rendered at {props.timestamp as string}</small>
	</section>
)

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

type remoteResolver struct {
	client *http.Client
	cache  sync.Map
}

func newRemoteResolver() *remoteResolver {
	return &remoteResolver{client: &http.Client{Timeout: 15 * time.Second}}
}

func (r *remoteResolver) Plugin() api.Plugin {
	aliases := map[string]string{
		"react":                 fmt.Sprintf("https://esm.sh/react@%s?dev", reactVersion),
		"react/jsx-runtime":     fmt.Sprintf("https://esm.sh/react@%s/jsx-runtime?dev", reactVersion),
		"react/jsx-dev-runtime": fmt.Sprintf("https://esm.sh/react@%s/jsx-dev-runtime?dev", reactVersion),
		"react-dom/server":      fmt.Sprintf("https://esm.sh/react-dom@%s/server?dev", reactVersion),
		"react-dom/client":      fmt.Sprintf("https://esm.sh/react-dom@%s/client?dev", reactVersion),
	}

	return api.Plugin{
		Name: "remote-react",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: "^https?://"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				return api.OnResolveResult{Path: args.Path, Namespace: "http-url"}, nil
			})

			build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				if target, ok := aliases[args.Path]; ok {
					return api.OnResolveResult{Path: target, Namespace: "http-url"}, nil
				}

				if args.Importer != "" && strings.HasPrefix(args.Importer, "http") {
					base, err := url.Parse(args.Importer)
					if err != nil {
						return api.OnResolveResult{}, err
					}

					if strings.HasPrefix(args.Path, "./") || strings.HasPrefix(args.Path, "../") {
						resolved := base.ResolveReference(&url.URL{Path: args.Path})
						return api.OnResolveResult{Path: resolved.String(), Namespace: "http-url"}, nil
					}

					if strings.HasPrefix(args.Path, "/") {
						resolved := &url.URL{
							Scheme: base.Scheme,
							Host:   base.Host,
							Path:   args.Path,
						}
						return api.OnResolveResult{Path: resolved.String(), Namespace: "http-url"}, nil
					}
				}

				return api.OnResolveResult{}, fmt.Errorf("unable to resolve %q", args.Path)
			})

			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "http-url"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				if cached, ok := r.cache.Load(args.Path); ok {
					text := cached.(string)
					return api.OnLoadResult{Contents: &text, Loader: api.LoaderJS}, nil
				}

				resp, err := r.client.Get(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				defer resp.Body.Close()
				if resp.StatusCode >= http.StatusBadRequest {
					return api.OnLoadResult{}, fmt.Errorf("fetch %s failed with %d", args.Path, resp.StatusCode)
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				text := string(body)
				r.cache.Store(args.Path, text)
				return api.OnLoadResult{Contents: &text, Loader: api.LoaderJS}, nil
			})
		},
	}
}
