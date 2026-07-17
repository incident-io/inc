package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestMain pins the environment before any test runs: config.Load is
// memoized per process, so the first command executed would otherwise cache
// whatever config file and INCIDENT_* variables the developer's shell has.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "inc-cmd-test")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", tmp)
	_ = os.Setenv("INCIDENT_API_KEY", "")
	_ = os.Setenv("INCIDENT_API_URL", "")
	_ = os.Setenv("INCIDENT_DEFAULT_OUTPUT", "")
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// cmdResult is what a command run produced.
type cmdResult struct {
	exit   int
	stdout string
	stderr string
}

// runCommand executes the CLI with the given args, capturing stdout, stderr,
// and the exit code. Flag state is reset afterwards so tests don't leak
// values into each other. Not safe for t.Parallel: it swaps process-global
// stdout/stderr and mutates the shared command tree.
func runCommand(t *testing.T, args ...string) cmdResult {
	t.Helper()

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wOut, wErr

	rootCmd.SetArgs(args)
	exit := Execute()

	os.Stdout, os.Stderr = oldOut, oldErr
	_ = wOut.Close()
	_ = wErr.Close()
	outB, _ := io.ReadAll(rOut)
	errB, _ := io.ReadAll(rErr)

	resetFlags(rootCmd)
	return cmdResult{exit: exit, stdout: string(outB), stderr: string(errB)}
}

// resetFlags restores every flag in the command tree to its default so one
// test's flags don't bleed into the next Execute call.
func resetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// recordedRequest captures one request the stub server received.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// stubServer serves canned responses keyed by "METHOD /path" and records
// every request. Responses for a key form a queue: each request pops the
// next one, and the final response keeps serving once the queue is down to
// one (so retries and repeat calls don't exhaust it). Unmatched requests get
// a 404 with an empty JSON object.
type stubServer struct {
	*httptest.Server
	requests  []recordedRequest
	responses map[string][]stubResponse
}

type stubResponse struct {
	status  int
	body    string
	headers map[string]string
}

func newStubServer(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{responses: map[string][]stubResponse{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.requests = append(s.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   string(body),
		})

		key := r.Method + " " + r.URL.Path
		queue := s.responses[key]
		if len(queue) == 0 {
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{}`)
			return
		}
		resp := queue[0]
		if len(queue) > 1 {
			s.responses[key] = queue[1:]
		}
		for k, v := range resp.headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		if resp.status != 0 {
			w.WriteHeader(resp.status)
		}
		_, _ = fmt.Fprint(w, resp.body)
	}))
	t.Cleanup(s.Close)
	return s
}

// respond queues canned 200 responses for "METHOD /path", one per call.
func (s *stubServer) respond(methodPath string, bodies ...string) {
	for _, b := range bodies {
		s.responses[methodPath] = append(s.responses[methodPath], stubResponse{body: b})
	}
}

// respondStatus queues a canned response with a status code and headers.
func (s *stubServer) respondStatus(methodPath string, status int, body string, headers map[string]string) {
	s.responses[methodPath] = append(s.responses[methodPath], stubResponse{status: status, body: body, headers: headers})
}

// args prefixes the standard auth flags for talking to the stub.
func (s *stubServer) args(rest ...string) []string {
	return append(rest, "--api-key", "test-key", "--api-url", s.URL)
}

// mustJSON fails the test unless s parses as JSON, returning the parsed value.
func mustJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("expected valid JSON, got error %v in: %s", err, s)
	}
	return v
}
