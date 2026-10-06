// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package livelog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drone/runner-go/client"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/logstream"
)

func TestLineWriterSingle(t *testing.T) {
	client := new(mockClient)
	w := New(client, "1", "1", nil, false)
	w.SetInterval(time.Duration(0))
	w.num = 4
	w.Write([]byte("foo\nbar\n")) // nolint:errcheck

	a := w.pending
	b := []*logstream.Line{
		{Number: 4, Message: "foo\n"},
		{Number: 5, Message: "bar\n"},
	}
	if err := compare(a, b); err != nil {
		t.Fail()
		fmt.Print(a)
		t.Log(err)
	}

	w.Close()
	a = client.uploaded
	if err := compare(a, b); err != nil {
		t.Fail()
		t.Log(err)
	}
}

func TestLineWriterSingleWithTrimNewLineSuffixEnabled(t *testing.T) {
	client := new(mockClient)
	w := New(client, "1", "1", nil, true)
	w.SetInterval(time.Duration(0))
	w.num = 4
	w.Write([]byte("foo\nbar\n")) // nolint:errcheck

	a := w.pending
	b := []*logstream.Line{
		{Number: 4, Message: "foo"},
		{Number: 5, Message: "bar"},
	}
	if err := compare(a, b); err != nil {
		t.Fail()
		fmt.Print(a)
		t.Log(err)
	}

	w.Close()
	a = client.uploaded
	if err := compare(a, b); err != nil {
		t.Fail()
		t.Log(err)
	}
}

func compare(a, b []*logstream.Line) error {
	if len(a) != len(b) {
		return fmt.Errorf("expected size: %d, actual: %d", len(a), len(b))
	}

	for i := 0; i < len(a); i++ {
		if a[i].Number != b[i].Number {
			return fmt.Errorf("expected number: %d, actual: %d", a[i].Number, b[i].Number)
		}
		if a[i].Message != b[i].Message {
			return fmt.Errorf("expected message: %s, actual: %s", a[i].Message, b[i].Message)
		}
	}
	return nil
}

type mockClient struct {
	client.Client
	lines       []*logstream.Line
	uploaded    []*logstream.Line
	openErr     error
	writeErr    error
	closeErr    error
	uploadErr   error
	closeCalls  int32
	uploadCalls int32
}

func (m *mockClient) Upload(ctx context.Context, key string, lines []*logstream.Line) error {
	atomic.AddInt32(&m.uploadCalls, 1)
	m.uploaded = lines
	return m.uploadErr
}

func (m *mockClient) Open(ctx context.Context, key string) error {
	return m.openErr
}

// Close closes the data stream.
func (m *mockClient) Close(ctx context.Context, key string) error {
	atomic.AddInt32(&m.closeCalls, 1)
	return m.closeErr
}

// Write writes logs to the data stream.
func (m *mockClient) Write(ctx context.Context, key string, lines []*logstream.Line) error {
	m.lines = append(m.lines, lines...)
	return m.writeErr
}

func TestRecordOp_FailedRPCDoesNotAddLatency(t *testing.T) {
	ops := []string{"open", "write", "close", "upload"}
	for _, op := range ops {
		w := &Writer{}
		w.recordOp(op, errors.New("boom"), 50*time.Millisecond)
		s := statsFor(w, op)
		if s.Count != 1 || s.ErrorCount != 1 || s.LatencyMs != 0 {
			t.Fatalf("%s failed RPC: got count=%d errorCount=%d latencyMs=%d", op, s.Count, s.ErrorCount, s.LatencyMs)
		}
	}
}

func TestRecordOp_SuccessOnlyLatencyMix(t *testing.T) {
	w := &Writer{}
	w.recordOp("write", nil, 20*time.Millisecond)
	w.recordOp("write", errors.New("boom"), 50*time.Millisecond)
	w.recordOp("write", nil, 10*time.Millisecond)
	s := w.stats.Write
	if s.Count != 3 || s.ErrorCount != 1 || s.LatencyMs != 30 {
		t.Fatalf("got count=%d errorCount=%d latencyMs=%d want 3/1/30", s.Count, s.ErrorCount, s.LatencyMs)
	}
}

func TestLogServiceOpStatsJSONHasNoBytes(t *testing.T) {
	raw, err := json.Marshal(api.LogServiceOpStats{Count: 1, ErrorCount: 1, LatencyMs: 10})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "bytes") {
		t.Fatalf("json must not include bytes: %s", raw)
	}
}

func TestOpenWriteCloseUpload_FailedRPCCounts(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		mc := &mockClient{openErr: errors.New("boom")}
		w := New(mc, "k", "n", nil, false)
		if err := w.Open(); err == nil {
			t.Fatal("expected open error")
		}
		s := w.LogServiceStats().Open
		if s.Count != 1 || s.ErrorCount != 1 || s.LatencyMs != 0 {
			t.Fatalf("open stats count=%d errorCount=%d latencyMs=%d", s.Count, s.ErrorCount, s.LatencyMs)
		}
	})
	t.Run("write", func(t *testing.T) {
		mc := &mockClient{writeErr: errors.New("boom")}
		w := New(mc, "k", "n", nil, false)
		w.SetInterval(time.Hour)
		if err := w.Open(); err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := w.Write([]byte("line\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := w.flush(); err == nil {
			t.Fatal("expected flush error")
		}
		s := w.LogServiceStats().Write
		if s.Count != 1 || s.ErrorCount != 1 || s.LatencyMs != 0 {
			t.Fatalf("write stats count=%d errorCount=%d latencyMs=%d", s.Count, s.ErrorCount, s.LatencyMs)
		}
		_ = w.Close()
	})
	t.Run("close", func(t *testing.T) {
		mc := &mockClient{closeErr: errors.New("boom")}
		w := New(mc, "k", "n", nil, false)
		w.SetInterval(time.Hour)
		if err := w.Open(); err != nil {
			t.Fatalf("open: %v", err)
		}
		_ = w.Close()
		s := w.LogServiceStats().Close
		if s.Count != 1 || s.ErrorCount != 1 || s.LatencyMs != 0 {
			t.Fatalf("close stats count=%d errorCount=%d latencyMs=%d", s.Count, s.ErrorCount, s.LatencyMs)
		}
	})
	t.Run("upload", func(t *testing.T) {
		mc := &mockClient{uploadErr: errors.New("boom")}
		w := New(mc, "k", "n", nil, false)
		w.SetInterval(time.Hour)
		if err := w.Open(); err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := w.Close(); err == nil {
			t.Fatal("expected upload error from Close")
		}
		s := w.LogServiceStats().Upload
		if s.Count != 1 || s.ErrorCount != 1 || s.LatencyMs != 0 {
			t.Fatalf("upload stats count=%d errorCount=%d latencyMs=%d", s.Count, s.ErrorCount, s.LatencyMs)
		}
	})
}

func statsFor(w *Writer, op string) logstream.OpStats {
	switch op {
	case "open":
		return w.stats.Open
	case "write":
		return w.stats.Write
	case "close":
		return w.stats.Close
	case "upload":
		return w.stats.Upload
	default:
		return logstream.OpStats{}
	}
}

// TestWriter_CloseIsIdempotent is the timeout-path regression: a second Close()
// must not re-run upload()+client.Close or double-count log_service_stats.
func TestWriter_CloseIsIdempotent(t *testing.T) {
	uploadErr := errors.New("upload boom")
	mc := &mockClient{uploadErr: uploadErr}
	w := New(mc, "k", "n", nil, false)
	w.SetInterval(time.Hour)
	if err := w.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	first := w.Close()
	second := w.Close()
	if first != uploadErr {
		t.Fatalf("first Close: got %v want %v", first, uploadErr)
	}
	if second != first {
		t.Fatalf("second Close: got %v want same as first %v", second, first)
	}
	if got := atomic.LoadInt32(&mc.uploadCalls); got != 1 {
		t.Fatalf("upload RPCs: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&mc.closeCalls); got != 1 {
		t.Fatalf("close RPCs: got %d want 1", got)
	}
	s := w.LogServiceStats()
	if s.Upload.Count != 1 || s.Upload.ErrorCount != 1 {
		t.Fatalf("upload stats count=%d errorCount=%d want 1/1", s.Upload.Count, s.Upload.ErrorCount)
	}
	if s.Close.Count != 1 || s.Close.ErrorCount != 0 {
		t.Fatalf("close stats count=%d errorCount=%d want 1/0", s.Close.Count, s.Close.ErrorCount)
	}
}

func TestWriter_CloseConcurrentIsIdempotent(t *testing.T) {
	mc := &mockClient{}
	w := New(mc, "k", "n", nil, false)
	w.SetInterval(time.Hour)
	if err := w.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = w.Close()
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&mc.uploadCalls); got != 1 {
		t.Fatalf("upload RPCs: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&mc.closeCalls); got != 1 {
		t.Fatalf("close RPCs: got %d want 1", got)
	}
	s := w.LogServiceStats()
	if s.Upload.Count != 1 || s.Close.Count != 1 {
		t.Fatalf("stats upload=%d close=%d want 1/1", s.Upload.Count, s.Close.Count)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("closer %d: %v", i, err)
		}
	}
}
