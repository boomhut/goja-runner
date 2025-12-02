package bundler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// ReactOptions controls how the React server/client bundles are generated.
type ReactOptions struct {
	ReactVersion string
	SSREntry     string
	ClientEntry  string
}

// ReactBundles contains the compiled server and client bundles.
type ReactBundles struct {
	SSR    string
	Client string
}

const defaultReactVersion = "18.3.1"

// BuildReactBundles produces bundled JavaScript suitable for SSR and
// client-side hydration. The entry points should export `renderApp` on the
// server side and call `hydrateRoot` on the client side.
func BuildReactBundles(opts ReactOptions) (*ReactBundles, error) {
	if strings.TrimSpace(opts.SSREntry) == "" {
		return nil, errors.New("ssr entry is required")
	}
	if strings.TrimSpace(opts.ClientEntry) == "" {
		return nil, errors.New("client entry is required")
	}

	reactVersion := opts.ReactVersion
	if reactVersion == "" {
		reactVersion = defaultReactVersion
	}

	resolver := newRemoteResolver(reactVersion)

	ssr, err := buildBundle(opts.SSREntry, "app-ssr.tsx", api.PlatformNode, resolver)
	if err != nil {
		return nil, fmt.Errorf("bundle ssr: %w", err)
	}

	client, err := buildBundle(opts.ClientEntry, "app-client.tsx", api.PlatformBrowser, resolver)
	if err != nil {
		return nil, fmt.Errorf("bundle client: %w", err)
	}

	return &ReactBundles{SSR: ssr, Client: client}, nil
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

type remoteResolver struct {
	client       *http.Client
	cache        sync.Map
	reactVersion string
}

func newRemoteResolver(reactVersion string) *remoteResolver {
	return &remoteResolver{
		client:       &http.Client{Timeout: 15 * time.Second},
		reactVersion: reactVersion,
	}
}

func (r *remoteResolver) Plugin() api.Plugin {
	aliases := map[string]string{
		"react":                 fmt.Sprintf("https://esm.sh/react@%s?dev", r.reactVersion),
		"react/jsx-runtime":     fmt.Sprintf("https://esm.sh/react@%s/jsx-runtime?dev", r.reactVersion),
		"react/jsx-dev-runtime": fmt.Sprintf("https://esm.sh/react@%s/jsx-dev-runtime?dev", r.reactVersion),
		"react-dom/server":      fmt.Sprintf("https://esm.sh/react-dom@%s/server?dev", r.reactVersion),
		"react-dom/client":      fmt.Sprintf("https://esm.sh/react-dom@%s/client?dev", r.reactVersion),
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
