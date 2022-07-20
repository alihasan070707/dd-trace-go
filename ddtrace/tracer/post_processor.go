// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package tracer

import (
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

var _ ddtrace.ReadWriteSpan = (*readWriteSpan)(nil)

// readWriteSpan wraps span and implements the ddtrace.ReadWriteSpan interface.
type readWriteSpan struct {
	*span
}

// Tag returns the tag value held by the given key.
func (s readWriteSpan) Tag(key string) interface{} {
	s.Lock()
	defer s.Unlock()

	switch key {
	// String.
	case ext.SpanName:
		return s.Name
	case ext.ServiceName:
		return s.Service
	case ext.ResourceName:
		return s.Resource
	case ext.SpanType:
		return s.Type
	// Bool.
	case ext.AnalyticsEvent:
		return s.Metrics[ext.EventSampleRate] == 1.0
	case ext.ManualDrop:
		return s.Metrics[keySamplingPriority] == -1
	case ext.ManualKeep:
		return s.Metrics[keySamplingPriority] == 2
	// Metrics.
	case ext.SamplingPriority:
		if val, ok := s.Metrics[keySamplingPriority]; ok {
			return val
		}
		return nil
	case keySamplingPriority:
		if val, ok := s.Metrics[keySamplingPriority]; ok {
			return val
		}
		return nil
	}
	if val, ok := s.Meta[key]; ok {
		return val
	}
	if val, ok := s.Metrics[key]; ok {
		return val
	}
	return nil
}

// GetName returns the operation name of s.
func (s readWriteSpan) GetName() string {
	s.Lock()
	defer s.Unlock()

	return s.Name
}

// GetService returns the service name of s.
func (s readWriteSpan) GetService() string {
	s.Lock()
	defer s.Unlock()

	return s.Service
}

// GetResource returns the resource name of s.
func (s readWriteSpan) GetResource() string {
	s.Lock()
	defer s.Unlock()

	return s.Resource
}

// GetType returns the type of s.
func (s readWriteSpan) GetType() string {
	s.Lock()
	defer s.Unlock()

	return s.Type
}

// GetDuration returns the duration of s.
func (s readWriteSpan) GetDuration() int64 {
	s.Lock()
	defer s.Unlock()

	return s.Duration
}

// IsError reports wether s is an error.
func (s readWriteSpan) IsError() bool {
	s.Lock()
	defer s.Unlock()

	return s.Error == 1
}

// ErrorMessage returns the error message of s.
func (s readWriteSpan) ErrorMessage() string {
	s.Lock()
	defer s.Unlock()

	return s.Meta[ext.ErrorMsg]
}

// ErrorType returns the error type of s.
func (s readWriteSpan) ErrorType() string {
	s.Lock()
	defer s.Unlock()

	return s.Meta[ext.ErrorType]
}

// ErrorStack returns the error stack of s.
func (s readWriteSpan) ErrorStack() string {
	s.Lock()
	defer s.Unlock()

	return s.Meta[ext.ErrorStack]
}

// ErrorDetails returns the error details of s.
func (s readWriteSpan) ErrorDetails() string {
	s.Lock()
	defer s.Unlock()

	return s.Meta[ext.ErrorDetails]
}

// SetOperationName is a no-op.
func (s readWriteSpan) SetOperationName(operationName string) {
	s.SetTag(ext.SpanName, operationName)
}

// SetTag adds a set of key/value metadata to the span. Setting metric aggregator tags
// (name, env, service, version, resource, http.status_code and keyMeasured) or modifying
// the sampling priority in the processor is not allowed.
func (s readWriteSpan) SetTag(key string, value interface{}) {
	s.Lock()
	defer s.Unlock()

	switch key {
	case ext.SpanName, ext.SpanType, ext.ResourceName, ext.ServiceName, ext.HTTPCode, ext.Environment, keyMeasured, keyTopLevel, ext.AnalyticsEvent, ext.EventSampleRate:
		log.Debug("Setting the tag %v in the processor is not allowed", key)
		return
	// Returning is not necessary, as the call to setSamplingPriorityLocked is
	// a no-op on finished spans. Adding this case for the purpose of logging
	// that this is not allowed.
	case ext.ManualKeep, ext.ManualDrop, ext.SamplingPriority, keySamplingPriority:
		log.Debug("Setting sampling priority tag %v in the processor is not allowed", key)
		return
	default:
		s.setTagLocked(key, value)
	}
}

// runProcessor pushes finished spans from a trace to the processor, and reports
// whether the trace should be dropped.
func (tr *tracer) runProcessor(spans []*span) bool {
	if tr.config.postProcessor == nil {
		return true
	}
	return tr.config.postProcessor(newReadWriteSpanSlice(spans))
}

// newReadWriteSpanSlice copies the elements of slice spans to the
// destination slice of type ddtrace.ReadWriteSpan to be fed to the processor.
func newReadWriteSpanSlice(spans []*span) []ddtrace.ReadWriteSpan {
	rwSlice := make([]ddtrace.ReadWriteSpan, len(spans))
	for i, span := range spans {
		rwSlice[i] = readWriteSpan{span}
	}
	return rwSlice
}
