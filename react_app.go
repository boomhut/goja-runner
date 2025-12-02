package jsrunner

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/boomhut/goja-runner/internal/bundler"
)

// ReactAppOptions configures the creation of a ReactApp helper.
type ReactAppOptions struct {
	// Runner allows supplying an existing Runner. When nil, a new runner is
	// created using RunnerOptions.
	Runner *Runner

	// RunnerOptions are applied when Runner is nil.
	RunnerOptions []Option

	// Polyfills are executed prior to loading the bundled React code. Use
	// this to install globals like TextEncoder/TextDecoder.
	Polyfills []string

	// SSREntry and ClientEntry contain the TypeScript/JSX source fed to
	// esbuild. These must define the renderApp function (server) and the
	// hydrateRoot bootstrap (client).
	SSREntry    string
	ClientEntry string

	// ReactVersion controls which React release is fetched from esm.sh.
	// Defaults to a sensible version when empty.
	ReactVersion string
}

// ReactApp wires a Runner together with a bundled React application so it can
// render HTML on the server while exposing a hydration bundle for browsers.
type ReactApp struct {
	runner       *Runner
	clientBundle string
	mu           sync.Mutex
}

// NewReactApp bundles the supplied entry points and installs them into the
// provided (or newly created) Runner. The resulting ReactApp can render props
// via renderApp(props) and expose the compiled client bundle.
func NewReactApp(opts ReactAppOptions) (*ReactApp, error) {
	if strings.TrimSpace(opts.SSREntry) == "" {
		return nil, errors.New("react ssr entry is required")
	}
	if strings.TrimSpace(opts.ClientEntry) == "" {
		return nil, errors.New("react client entry is required")
	}

	r := opts.Runner
	if r == nil {
		r = New(opts.RunnerOptions...)
	}

	for idx, script := range opts.Polyfills {
		if strings.TrimSpace(script) == "" {
			continue
		}
		if err := r.LoadScriptString(script); err != nil {
			return nil, fmt.Errorf("load polyfill[%d]: %w", idx, err)
		}
	}

	bundles, err := bundler.BuildReactBundles(bundler.ReactOptions{
		ReactVersion: opts.ReactVersion,
		SSREntry:     opts.SSREntry,
		ClientEntry:  opts.ClientEntry,
	})
	if err != nil {
		return nil, err
	}

	if err := r.LoadScriptString(bundles.SSR); err != nil {
		return nil, fmt.Errorf("load SSR bundle: %w", err)
	}

	if err := assertGlobalExists(r, "renderApp"); err != nil {
		return nil, fmt.Errorf("renderApp not defined: %w", err)
	}

	return &ReactApp{runner: r, clientBundle: bundles.Client}, nil
}

// Render executes renderApp inside the underlying Runner with the supplied
// props and returns the HTML markup.
func (ra *ReactApp) Render(props map[string]interface{}) (string, error) {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	ra.runner.SetGlobal("SERVER_PROPS", props)

	markup, err := ra.runner.Eval("renderApp(SERVER_PROPS)")
	if err != nil {
		return "", fmt.Errorf("renderApp failed: %w", err)
	}

	return ExportString(markup), nil
}

// ClientBundle returns the compiled browser bundle that hydrates the app.
func (ra *ReactApp) ClientBundle() string {
	return ra.clientBundle
}

// Runner exposes the underlying jsrunner.Runner for advanced customization.
func (ra *ReactApp) Runner() *Runner {
	return ra.runner
}

func assertGlobalExists(r *Runner, name string) error {
	result, err := r.Eval(fmt.Sprintf("typeof this['%s'] !== 'undefined'", name))
	if err != nil {
		return err
	}
	if !ExportBool(result) {
		return fmt.Errorf("global %s is undefined", name)
	}
	return nil
}
