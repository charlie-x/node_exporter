// Copyright 2025 The Prometheus Authors / charliex
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !nogpu
// +build !nogpu

package collector

import (
	"errors"
	"fmt"
	"strconv"

	"log/slog"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"
)

// gpuCollector collects NVIDIA GPU metrics using NVML
type gpuCollector struct {
	logger *slog.Logger

	// Prometheus metric descriptors.
	gpuUtilizationDesc *prometheus.Desc
	gpuTemperatureDesc *prometheus.Desc
	gpuMemoryTotalDesc *prometheus.Desc
	gpuMemoryUsedDesc  *prometheus.Desc
	gpuMemoryFreeDesc  *prometheus.Desc
	gpuInfoDesc        *prometheus.Desc
}

// namespace and subsystem for the metrics
const (
	gpuCollectorSubsystem = "gpu"
)

// init and add the collector
func init() {
	registerCollector("nvidia", defaultEnabled, NewGPUCollector)
}

// NewGPUCollector creates a new GPU collector and initialises NVML
// returns an error if NVML cannot be initialised
func NewGPUCollector(logger *slog.Logger) (Collector, error) {
	// initialise NVML
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("could not initialise NVML: %v", ret)
	}

	// create metric descriptors
	g := &gpuCollector{
		logger: logger,
		gpuUtilizationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "utilisation_percentage"),
			"GPU utilisation in percent.",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
		gpuTemperatureDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "temperature_celsius"),
			"GPU temperature in Celsius.",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
		gpuMemoryTotalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "memory_total_bytes"),
			"Total GPU memory in bytes.",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
		gpuMemoryUsedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "memory_used_bytes"),
			"Used GPU memory in bytes.",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
		gpuMemoryFreeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "memory_free_bytes"),
			"Free GPU memory in bytes.",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
		gpuInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, gpuCollectorSubsystem, "info"),
			"Static GPU information (e.g. index and name).",
			[]string{"gpu_index", "gpu_name"}, nil,
		),
	}

	return g, nil
}

// update collects GPU metrics using NVML and sends them to the prometheus metric channel
func (g *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	// retrieve the number of NVIDIA GPUs
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		g.logger.Error("failed to get GPU count", "return", ret)
		return fmt.Errorf("could not retrieve GPU count: %v", ret)
	}
	if count == 0 {
		return errors.New("no NVIDIA GPUs found")
	}

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			g.logger.Warn("failed to get handle for GPU device", "gpu_index", i, "return", ret)
			continue
		}

		// retrieve the GPU name
		name, ret := device.GetName()
		if ret != nvml.SUCCESS {
			g.logger.Warn("failed to get GPU name", "gpu_index", i, "return", ret)
			name = "unknown"
		}

		// retrieve GPU utilization rates
		util, ret := device.GetUtilizationRates()
		if ret != nvml.SUCCESS {
			g.logger.Warn("failed to get GPU utilization", "gpu_index", i, "return", ret)
			continue
		}

		// retrieve GPU temperature
		temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
		if ret != nvml.SUCCESS {
			g.logger.Warn("failed to get GPU temperature", "gpu_index", i, "return", ret)
			continue
		}

		// retrieve GPU memory info
		mem, ret := device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			g.logger.Warn("failed to get GPU memory info", "gpu_index", i, "return", ret)
			continue
		}

		gpuIndex := strconv.Itoa(i)

		gpuUtilization := float64(util.Gpu)

		// export metrics
		ch <- prometheus.MustNewConstMetric(
			g.gpuUtilizationDesc,
			prometheus.GaugeValue,
			gpuUtilization,
			gpuIndex, name,
		)
		ch <- prometheus.MustNewConstMetric(
			g.gpuTemperatureDesc,
			prometheus.GaugeValue,
			float64(temp),
			gpuIndex, name,
		)
		ch <- prometheus.MustNewConstMetric(
			g.gpuMemoryTotalDesc,
			prometheus.GaugeValue,
			float64(mem.Total),
			gpuIndex, name,
		)
		ch <- prometheus.MustNewConstMetric(
			g.gpuMemoryUsedDesc,
			prometheus.GaugeValue,
			float64(mem.Used),
			gpuIndex, name,
		)
		ch <- prometheus.MustNewConstMetric(
			g.gpuMemoryFreeDesc,
			prometheus.GaugeValue,
			float64(mem.Free),
			gpuIndex, name,
		)
		// export a static metric with GPU information
		ch <- prometheus.MustNewConstMetric(
			g.gpuInfoDesc,
			prometheus.GaugeValue,
			1,
			gpuIndex, name,
		)
	}

	return nil
}
