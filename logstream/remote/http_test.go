package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harness/harness-docker-runner/logstream"
)

const blobUploadTimeout = 60 * time.Second

type capturedRequest struct {
	method      string
	url         string
	hasDeadline bool
	remaining   time.Duration
}

type fakeTransport struct {
	mu       sync.Mutex
	captured []capturedRequest
	respond  func(req *http.Request, attempt int) (*http.Response, error)
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := capturedRequest{method: req.Method, url: req.URL.String()}
	if deadline, ok := req.Context().Deadline(); ok {
		c.hasDeadline = true
		c.remaining = time.Until(deadline)
	}

	f.mu.Lock()
	f.captured = append(f.captured, c)
	attempt := len(f.captured)
	respond := f.respond
	f.mu.Unlock()

	if respond != nil {
		return respond(req, attempt)
	}
	return jsonResponse(http.StatusOK, "{}"), nil
}

func (f *fakeTransport) requests() []capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedRequest(nil), f.captured...)
}

func jsonResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func stallUntilDone(req *http.Request, _ int) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func newTestClient(resilient bool, ft *fakeTransport) *HTTPClient {
	c := NewHTTPClient("http://log-service.test", "acct", "tok", false, false, resilient)
	c.Client = &http.Client{Transport: ft}
	return c
}

func assertDeadline(t *testing.T, got capturedRequest, want time.Duration) {
	t.Helper()
	if !got.hasDeadline {
		t.Fatalf("%s %s: no deadline on request context, want ~%v", got.method, got.url, want)
	}
	const slack = time.Second
	if got.remaining > want || got.remaining < want-slack {
		t.Fatalf("%s %s: deadline %v away, want ~%v", got.method, got.url, got.remaining, want)
	}
}

func assertNoDeadline(t *testing.T, got capturedRequest) {
	t.Helper()
	if got.hasDeadline {
		t.Fatalf("%s %s: request context bounded at %v, want no deadline", got.method, got.url, got.remaining)
	}
}

func assertGaveUpWithinBackoff(t *testing.T, elapsed, budget time.Duration) {
	t.Helper()
	const slack = 2 * time.Second
	if elapsed > budget+slack {
		t.Fatalf("gave up after %v, want no more than the %v backoff budget", elapsed, budget)
	}
	if elapsed < budget*2/5 {
		t.Fatalf("gave up after %v, far short of the %v backoff budget: retries look broken", elapsed, budget)
	}
}

func assertSingleRequest(t *testing.T, ft *fakeTransport) capturedRequest {
	t.Helper()
	reqs := ft.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want exactly 1", len(reqs))
	}
	return reqs[0]
}

type gatedCall struct {
	name    string
	timeout time.Duration
	call    func(context.Context, *HTTPClient) error
}

func gatedCalls() []gatedCall {
	return []gatedCall{
		{
			name:    "Open",
			timeout: openStreamTimeout,
			call:    func(ctx context.Context, c *HTTPClient) error { return c.Open(ctx, "key") },
		},
		{
			name:    "Close",
			timeout: closeStreamTimeout,
			call:    func(ctx context.Context, c *HTTPClient) error { return c.Close(ctx, "key") },
		},
		{
			name:    "uploadLink",
			timeout: uploadLinkTimeout,
			call: func(ctx context.Context, c *HTTPClient) error {
				_, err := c.uploadLink(ctx, "key")
				return err
			},
		},
		{
			name:    "uploadToRemoteStorage",
			timeout: blobUploadTimeout,
			call: func(ctx context.Context, c *HTTPClient) error {
				return c.uploadToRemoteStorage(ctx, "key", strings.NewReader("log line"))
			},
		},
		{
			name:    "uploadUsingLink",
			timeout: blobUploadTimeout,
			call: func(ctx context.Context, c *HTTPClient) error {
				return c.uploadUsingLink(ctx, "http://storage.test/blob", strings.NewReader("log line"))
			},
		},
	}
}

func TestFlagOn_BoundsEveryCall(t *testing.T) {
	t.Parallel()
	for _, tc := range gatedCalls() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ft := &fakeTransport{}
			if err := tc.call(context.Background(), newTestClient(true, ft)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertDeadline(t, assertSingleRequest(t, ft), tc.timeout)
		})
	}
}

func TestFlagOff_LeavesEveryCallUnbounded(t *testing.T) {
	t.Parallel()
	for _, tc := range gatedCalls() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ft := &fakeTransport{}
			if err := tc.call(context.Background(), newTestClient(false, ft)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertNoDeadline(t, assertSingleRequest(t, ft))
		})
	}
}

func TestWrite_UnboundedInBothFlagStates(t *testing.T) {
	t.Parallel()
	for _, resilient := range []bool{true, false} {
		resilient := resilient
		t.Run(map[bool]string{true: "flagOn", false: "flagOff"}[resilient], func(t *testing.T) {
			t.Parallel()
			ft := &fakeTransport{}
			lines := []*logstream.Line{{Level: "info", Message: "hello", Number: 0}}
			if err := newTestClient(resilient, ft).Write(context.Background(), "key", lines); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertNoDeadline(t, assertSingleRequest(t, ft))
		})
	}
}

func TestFlagOn_KeepsShorterCallerDeadline(t *testing.T) {
	t.Parallel()
	const callerTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), callerTimeout)
	defer cancel()

	ft := &fakeTransport{}
	if err := newTestClient(true, ft).Open(ctx, "key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDeadline(t, assertSingleRequest(t, ft), callerTimeout)
}

func TestGatedTimeoutValues(t *testing.T) {
	t.Parallel()
	values := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"openStreamTimeout", openStreamTimeout, 30 * time.Second},
		{"openStreamBackoff", openStreamBackoff, 10 * time.Second},
		{"closeStreamTimeout", closeStreamTimeout, 15 * time.Second},
		{"uploadLinkTimeout", uploadLinkTimeout, 10 * time.Second},
		{"uploadLinkBackoff", uploadLinkBackoff, 10 * time.Second},
	}
	for _, v := range values {
		if v.got != v.want {
			t.Errorf("%s = %v, want %v", v.name, v.got, v.want)
		}
	}
}

func TestNewHTTPClient_FlagPlumbing(t *testing.T) {
	t.Parallel()
	resilient := NewHTTPClient("http://e", "acct", "tok", true, false, true)
	if !resilient.LogResilient || !resilient.IndirectUpload || resilient.SkipVerify {
		t.Fatalf("got LogResilient=%v IndirectUpload=%v SkipVerify=%v, want true/true/false",
			resilient.LogResilient, resilient.IndirectUpload, resilient.SkipVerify)
	}

	plain := NewHTTPClient("http://e", "acct", "tok", false, true, false)
	if plain.LogResilient || plain.IndirectUpload || !plain.SkipVerify {
		t.Fatalf("got LogResilient=%v IndirectUpload=%v SkipVerify=%v, want false/false/true",
			plain.LogResilient, plain.IndirectUpload, plain.SkipVerify)
	}
}

func TestOpen_BackoffUnaffectedByFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the real openStreamBackoff")
	}
	t.Parallel()
	for _, resilient := range []bool{true, false} {
		resilient := resilient
		t.Run(map[bool]string{true: "flagOn", false: "flagOff"}[resilient], func(t *testing.T) {
			t.Parallel()
			ft := &fakeTransport{respond: func(*http.Request, int) (*http.Response, error) {
				return jsonResponse(http.StatusInternalServerError, `{"error_msg":"log service down"}`), nil
			}}

			start := time.Now()
			err := newTestClient(resilient, ft).Open(context.Background(), "key")
			elapsed := time.Since(start)

			var remoteErr *Error
			if !errors.As(err, &remoteErr) || remoteErr.Code != http.StatusInternalServerError {
				t.Fatalf("got err %v, want the 500 from the last attempt", err)
			}
			assertGaveUpWithinBackoff(t, elapsed, openStreamBackoff)
			if elapsed >= openStreamTimeout {
				t.Fatalf("gave up after %v, want the %v backoff to stop it well before the %v context timeout",
					elapsed, openStreamBackoff, openStreamTimeout)
			}
			if len(ft.requests()) < 2 {
				t.Fatalf("made %d requests, want retries", len(ft.requests()))
			}
			t.Logf("gave up after %d attempts in %v", len(ft.requests()), elapsed)
		})
	}
}

func TestUploadToRemoteStorage_FlagOn_TimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the real 60s upload timeout")
	}
	t.Parallel()
	ft := &fakeTransport{respond: stallUntilDone}

	start := time.Now()
	err := newTestClient(true, ft).uploadToRemoteStorage(context.Background(), "key", strings.NewReader("log line"))
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got err %v, want context.DeadlineExceeded", err)
	}
	assertDeadline(t, assertSingleRequest(t, ft), blobUploadTimeout)
	if elapsed < blobUploadTimeout || elapsed > blobUploadTimeout+5*time.Second {
		t.Fatalf("returned after %v, want ~%v", elapsed, blobUploadTimeout)
	}
}

func TestUploadUsingLink_FlagOn_BoundedByContextNotBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the real 60s upload timeout")
	}
	t.Parallel()
	ft := &fakeTransport{respond: stallUntilDone}

	start := time.Now()
	err := newTestClient(true, ft).uploadUsingLink(context.Background(), "http://storage.test/blob", strings.NewReader("log line"))
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got err %v, want context.DeadlineExceeded", err)
	}
	if elapsed < blobUploadTimeout || elapsed > blobUploadTimeout+5*time.Second {
		t.Fatalf("returned after %v, want ~%v", elapsed, blobUploadTimeout)
	}
}

func TestUploadUsingLink_FlagOff_GivesUpAtBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the real 60s backoff")
	}
	t.Parallel()
	transportErr := errors.New("connection refused")
	ft := &fakeTransport{respond: func(*http.Request, int) (*http.Response, error) {
		return nil, transportErr
	}}

	start := time.Now()
	err := newTestClient(false, ft).uploadUsingLink(context.Background(), "http://storage.test/blob", strings.NewReader("log line"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("got nil error, want the transport failure")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want a transport failure: flag off must not bound the context", err)
	}
	assertGaveUpWithinBackoff(t, elapsed, blobUploadTimeout)
	assertNoDeadline(t, ft.requests()[0])
	t.Logf("gave up after %d attempts in %v", len(ft.requests()), elapsed)
}
