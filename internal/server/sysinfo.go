package server

import (
	"fmt"

	"anytls/internal/api"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// collectSystemInfo 收集系统 CPU、内存、磁盘、交换区使用信息
func collectSystemInfo() (*api.NodeStatus, error) {
	status := &api.NodeStatus{}

	// CPU 使用率（整体，非阻塞）
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return nil, fmt.Errorf("获取 CPU 使用率失败: %w", err)
	}
	if len(cpuPercent) > 0 {
		status.CPU = cpuPercent[0]
	}

	// 内存使用
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("获取内存信息失败: %w", err)
	}
	status.Mem = api.ResourceUsage{
		Total: int64(vmStat.Total),
		Used:  int64(vmStat.Used),
	}

	// 交换区使用
	swapStat, err := mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("获取交换区信息失败: %w", err)
	}
	status.Swap = api.ResourceUsage{
		Total: int64(swapStat.Total),
		Used:  int64(swapStat.Used),
	}

	// 磁盘使用（根分区）
	diskStat, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("获取磁盘信息失败: %w", err)
	}
	status.Disk = api.ResourceUsage{
		Total: int64(diskStat.Total),
		Used:  int64(diskStat.Used),
	}

	return status, nil
}
