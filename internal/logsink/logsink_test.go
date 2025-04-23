// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
)

type logSinkSuite struct {
	testing.IsolationSuite

	states chan string
	closed int64
}

var _ = gc.Suite(&logSinkSuite{})

func (s *logSinkSuite) TestWriteWithNoBatching(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestWriteWithMultiline(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level: loggo.INFO,
		Message: `h
		
ello

wo

rld
`,
	})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "h\n\t\t\nello\n\nwo\n\nrld\n",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestWriteWithLargeBatching(c *gc.C) {
	// This forces the ticker to flush the batch.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestWriteWithLogsBatching(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(48) + 1) + 100

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for _, entry := range entries {
		sink.Write(entry)
	}

	// We should see 2 flushes, and flush via the remaining entries.
	s.expectNumOfFlushes(c, 2)
	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Timestamp,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestWriteWithLogsUnderBatchSize(c *gc.C) {
	// This leans on the timer to send all the logs.

	sink, buffer := s.newLogSink(c, 1000)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(100) + 1) + 100

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for _, entry := range entries {
		sink.Write(entry)
	}

	s.expectTick(c)
	s.expectMinNumOfFlushes(c, 1)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Timestamp,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestWriteLogsConcurrently(c *gc.C) {
	// Flood the sink with logs from multiple goroutines. We don't care about
	// the order of the logs, just that they all get written. All logs will be
	// localised to the original goroutine.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	total := 10000
	division := 100
	amount := total / division

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for i := range division {
		go func(i int, entries []loggo.Entry) {
			for _, entry := range entries {
				sink.Write(entry)
			}
		}(i, entries[i*amount:(i*amount)+amount])
	}

	// Wait for all the flushes to complete.
	s.expectNumOfFlushes(c, division)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Timestamp,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}

	// We can't guarantee the order of the entries written in the test, so we
	// need to sort them before comparing.
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Time.Before(lines[j].Time)
	})

	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogWithNoBatching(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogWithMultiline(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level: logger.INFO,
		Message: `h
		
ello

wo

rld
`}})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "h\n\t\t\nello\n\nwo\n\nrld\n",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogWithLargeBatching(c *gc.C) {
	// This forces the ticker to flush the batch.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogWithLogsBatching(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(48) + 1) + 100

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	sink.Log(entries)

	// We only expect 1 flush, as batching using the Log method, doesn't break
	// the logs into smaller chunks.
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Time,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogWithLogsUnderBatchSize(c *gc.C) {
	// This leans on the timer to send all the logs.

	sink, buffer := s.newLogSink(c, 1000)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(100) + 1) + 100

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	sink.Log(entries)

	s.expectTick(c)
	s.expectMinNumOfFlushes(c, 1)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Time,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogLogsConcurrently(c *gc.C) {
	// Flood the sink with logs from multiple goroutines. We don't care about
	// the order of the logs, just that they all get written. All logs will be
	// localised to the original goroutine.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	total := 10000
	division := 100
	amount := total / division

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	for i := range division {
		go func(i int, entries []logger.LogRecord) {
			sink.Log(entries)
		}(i, entries[i*amount:(i*amount)+amount])
	}

	// Wait for all the flushes to complete.
	s.expectNumOfFlushes(c, division)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Time,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
			},
			ModelUUID: "uuid",
		}
	}

	// We can't guarantee the order of the entries written in the test, so we
	// need to sort them before comparing.
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Time.Before(lines[j].Time)
	})

	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogAndWriteInterleaved(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(48) + 1) + 100

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for i, entry := range entries {
		if i%2 == 0 {
			sink.Write(entry)
		} else {
			sink.Log([]logger.LogRecord{{
				Time:      entry.Timestamp,
				ModelUUID: "uuid",
				Module:    entry.Module,
				Location:  fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
				Level:     logger.Level(entry.Level),
				Message:   entry.Message,
				Labels:    entry.Labels,
			}})
		}
	}

	// We should see 2 flushes, and flush via the remaining entries.
	s.expectNumOfFlushes(c, 2)
	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Time:     entry.Timestamp,
			Level:    level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)

	s.expectWriterClosed(c)
}

func (s *logSinkSuite) TestLogAndWriteInterleavedNoSynchronization(c *gc.C) {
	// Ensure that the internal states don't accientally synchronize the
	// interleaved writes and logs and prevent a race.

	buffer := new(bytes.Buffer)
	writer := &bufferCloser{Buffer: buffer, fn: func() {
		atomic.AddInt64(&s.closed, 1)
	}}

	sink := NewLogSink(writer, 50, time.Millisecond*100, clock.WallClock)
	defer workertest.DirtyKill(c, sink)

	total := 100

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Level:   loggo.INFO,
			Message: "h",
			Module:  "m",
		}
	}

	for i, entry := range entries {
		if i%2 == 0 {
			sink.Write(entry)
		} else {
			sink.Log([]logger.LogRecord{{
				Module:  entry.Module,
				Level:   logger.Level(entry.Level),
				Message: entry.Message,
			}})
		}
	}

	level, ok := logger.ParseLevelFromString(entries[0].Level.String())
	if !ok {
		c.Fatalf("failed to parse level %q", entries[0].Level.String())
	}

	// Calculate how many bytes we expect to be written.
	bytes, err := json.Marshal(&logger.LogRecord{
		Time:    entries[0].Timestamp,
		Level:   level,
		Message: entries[0].Message,
		Module:  entries[0].Module,
	})
	c.Assert(err, jc.ErrorIsNil)

	newLineLength := 1
	expectedWritten := (len(bytes) + newLineLength) * total

	// We have no synchronization points, so we need check if the buffer has
	// been written to.
	done := make(chan struct{})
	go func() {
		defer close(done)

		for {
			select {
			case <-time.After(testing.ShortWait):
				if writer.Written() == int64(expectedWritten) {
					return
				}

			case <-time.After(testing.LongWait):
				// We didn't see the buffer writes, so give up!
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for buffer to be written to")
	}

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logger.LogRecord, total)
	for k, entry := range entries {
		level, ok := logger.ParseLevelFromString(entry.Level.String())
		if !ok {
			c.Fatalf("failed to parse level %q", entry.Level.String())
		}

		expected[k] = logger.LogRecord{
			Level:   level,
			Message: entry.Message,
			Module:  entry.Module,
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) newLogSink(c *gc.C, batchSize int) (*LogSink, *bytes.Buffer) {
	s.states = make(chan string, 1)

	atomic.StoreInt64(&s.closed, 0)

	buffer := new(bytes.Buffer)
	writerCloser := &bufferCloser{Buffer: buffer, fn: func() {
		atomic.AddInt64(&s.closed, 1)
	}}

	sink := newLogSink(writerCloser, batchSize, time.Millisecond*100, clock.WallClock, s.states)
	return sink, buffer
}

func (s *logSinkSuite) expectFlush(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateFlushed)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *logSinkSuite) expectNumOfFlushes(c *gc.C, flushes int) {
	for {
		select {
		case state := <-s.states:
			if state == stateFlushed {
				flushes--
				if flushes == 0 {
					return
				}
			}
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for %d flushes", flushes)
		}
	}
}

func (s *logSinkSuite) expectMinNumOfFlushes(c *gc.C, expected int) {
	var flushes int
LOOP:
	for {
		select {
		case state := <-s.states:
			if state == stateFlushed {
				flushes++
			}
		case <-time.After(time.Second):
			break LOOP
		}
	}
	c.Assert(flushes >= expected, jc.IsTrue, gc.Commentf("expected more than 1 flush, got %d", flushes))
}

func (s *logSinkSuite) expectTick(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateTicked)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *logSinkSuite) expectWriterClosed(c *gc.C) {
	c.Assert(atomic.LoadInt64(&s.closed), gc.Equals, int64(1))
}

func parseLog(c *gc.C, reader io.Reader) []logger.LogRecord {
	var records []logger.LogRecord

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var record logger.LogRecord
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, jc.ErrorIsNil)
		records = append(records, record)
	}

	return records
}

type bufferCloser struct {
	*bytes.Buffer
	written int64
	fn      func()
}

// Write writes to the buffer and increments the written counter.
func (b *bufferCloser) Write(p []byte) (int, error) {
	written, err := b.Buffer.Write(p)
	if err != nil {
		return -1, err
	}

	atomic.AddInt64(&b.written, int64(written))

	return written, nil
}

// Written returns the number of bytes written to the buffer.
func (b *bufferCloser) Written() int64 {
	return atomic.LoadInt64(&b.written)
}

// Close closes the buffer and calls the close function.
func (b *bufferCloser) Close() error {
	b.fn()
	return nil
}
