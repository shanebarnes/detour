package main

import (
	"fmt"
	"time"

	"github.com/shanebarnes/goto/logger"
)

type Metrics struct {
	timeStartNs   int64
	timeReportNs  int64 // Last report time
	byteReport    int64
	reportIntNs   int64 // Report interval in nanoseconds
	reportIntByte int64
	byteCount     int64
	tag           string
	buffer        []string
}

func MetricsNew(reportIntNs int64, reportIntByte int64, tag string) *Metrics {
	metrics := new(Metrics)
	metrics.timeStartNs = time.Now().UnixNano()
	metrics.timeReportNs = metrics.timeStartNs
	metrics.reportIntNs = reportIntNs
	metrics.reportIntByte = reportIntByte
	metrics.tag = tag
	metrics.byteReport = 0
	metrics.byteCount = 0
	metrics.Add(0)
	return metrics
}

func (m *Metrics) Add(bytes int64) {
	now := time.Now().UnixNano()
	m.byteCount = m.byteCount + bytes
	m.byteReport = m.byteReport + bytes

	if (m.reportIntNs > 0 && now >= m.timeReportNs) || (m.reportIntByte > 0 && m.byteReport >= m.reportIntByte) {
		avgBps := float64(m.byteCount) * 8. * 1000000000. / float64(now-m.timeStartNs)
		m.buffer = append(m.buffer, fmt.Sprintf("%.6f,%s,%d,%d,%d,%.6f", float64(now)/1000000000., m.tag, (now-m.timeStartNs)/1000, bytes, m.byteCount, avgBps/1000000.))

		m.byteReport = 0
		for m.timeReportNs <= now {
			m.timeReportNs = m.timeReportNs + m.reportIntNs
		}
	}
}

func (m *Metrics) Dump() {
	m.timeReportNs = time.Now().UnixNano()
	m.Add(0)

	for _, record := range m.buffer {
		logger.PrintlnInfo(record)
	}
}
