// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"encoding/json"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/testing"
)

type LogRecordSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LogRecordSuite{})

func (s *LogRecordSuite) TestMarshall(c *gc.C) {
	rec := &logger.LogRecord{
		Time:      time.Date(2024, 1, 1, 9, 8, 7, 0, time.UTC),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "some-entity",
		Level:     2,
		Module:    "some-module",
		Location:  "some-location",
		Message:   "some-message",
		Labels:    map[string]string{"foo": "bar"},
	}
	data, err := json.Marshal(rec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, `{"timestamp":"2024-01-01T09:08:07Z","entity":"some-entity","level":"DEBUG","module":"some-module","location":"some-location","message":"some-message","labels":{"foo":"bar"}}`)
}

func (s *LogRecordSuite) TestMarshallRoundTrip(c *gc.C) {
	rec := &logger.LogRecord{
		Time:      time.Date(2024, 1, 1, 9, 8, 7, 0, time.UTC),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "some-entity",
		Level:     2,
		Module:    "some-module",
		Location:  "some-location",
		Message:   "some-message",
		Labels:    map[string]string{"foo": "bar"},
	}
	data, err := json.Marshal(rec)
	c.Assert(err, jc.ErrorIsNil)
	var got logger.LogRecord
	err = json.Unmarshal(data, &got)
	c.Assert(err, jc.ErrorIsNil)
	rec.ModelUUID = ""
	c.Assert(got, jc.DeepEquals, *rec)
}
