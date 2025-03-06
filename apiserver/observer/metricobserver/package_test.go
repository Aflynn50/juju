// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/metrics_collector_mock.go github.com/juju/juju/apiserver/observer/metricobserver MetricsCollector,SummaryVec
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/metrics_mock.go github.com/prometheus/client_golang/prometheus Summary

func Test(t *testing.T) {
	gc.TestingT(t)
}

func createMockMetrics(c *gc.C, labels interface{}) (*mocks.MockMetricsCollector, func()) {
	ctrl := gomock.NewController(c)

	summary := mocks.NewMockSummary(ctrl)
	summary.EXPECT().Observe(gomock.Any()).AnyTimes()

	summaryVec := mocks.NewMockSummaryVec(ctrl)
	summaryVec.EXPECT().With(labels).Return(summary).AnyTimes()

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)
	metricsCollector.EXPECT().APIRequestDuration().Return(summaryVec).AnyTimes()

	return metricsCollector, ctrl.Finish
}
