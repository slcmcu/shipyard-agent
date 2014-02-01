package utils

import (
	"os"
	"testing"
)

func TestGetCPUUsage(t *testing.T) {
	pid := os.Getpid()
	if usage := GetCPUUsage(pid); usage == -1 {
		t.Fatal("Error getting CPU Usage")
	}
}

func TestGetMemoryUsage(t *testing.T) {
	pid := os.Getpid()
	if usage := GetMemoryUsage(pid); usage == -1 {
		t.Fatal("Error getting Memory Usage")
	}
}
