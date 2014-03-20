package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type (
	Counter struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
		Unit  string `json:"unit"`
	}
	ContainerMetric struct {
		ContainerID string    `json:"container_id"`
		Counters    []Counter `json:"counters"`
		Type        string    `json:"type"`
	}
)

// Gets the CPU usage in percent for specified process id (pid)
func GetCPUUsage(pid int) int {
	up, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		log.Fatal("Unable to read /proc/uptime: ", err)
		return -1
	}
	stat, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		log.Fatal("Unable to read /proc: ", err)
		return -1
	}
	parts := strings.Split(string(stat), " ")
	cpuUsage := 0
	var (
		utime, _     = strconv.Atoi(parts[14])
		stime, _     = strconv.Atoi(parts[15])
		cutime, _    = strconv.Atoi(parts[16])
		cstime, _    = strconv.Atoi(parts[17])
		startTime, _ = strconv.Atoi(parts[22])
		totalTime    = utime + stime + cutime + cstime
		uptime, _    = strconv.Atoi(string(up))
		seconds      = uptime - (startTime / 1000)
	)
	if seconds > 0 {
		cpuUsage = 100 * ((totalTime / 1000) / seconds)
	}
	return cpuUsage
}

// Gets RSS (resident set size) in kilobytes for specified process id
func GetMemoryUsage(pid int) int {
	stat, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/status", pid))
	if err != nil {
		log.Fatal("Unable to read /proc: ", err)
		return -1
	}
	var res int
	for _, s := range strings.Split(string(stat), "\n") {
		if s == "" {
			continue
		}
		i := strings.Index(s, "VmRSS")
		if i > -1 {
			memParts := strings.Split(s, ":")
			rss := strings.Split(strings.TrimSpace(memParts[1]), " ")
			res, _ = strconv.Atoi(rss[0])
			break
		}
	}
	return res
}

// Check all running processes for a docker container and create a metric
func GetContainerMetric(pid int, containerID string) (ContainerMetric, error) {
	var metric ContainerMetric
	// hack: split on -n and -f to get container id
	//c := exec.Command("lxc-ps", "-n", containerID, "--", "o", "pid")
	c := exec.Command("ps", "-eo", "ppid,pid", "--no-headers")
	var cOut bytes.Buffer
	c.Stdout = &cOut
        err := c.Run()
	if err != nil {
		log.Printf("Erroring getting metrics for %s (PID %s): %s", containerID, pid, err)
		return metric, err
	}
	cpu := 0
	mem := 0
	for _, s := range strings.Split(cOut.String(), "\n") {
		f := strings.Fields(s)
                // skip blank lines
                if len(f) != 2 { continue }
                p, err := strconv.Atoi(f[1])
                if err != nil {
                    log.Printf("Error converting PID to in for metrics: %s", err)
                    continue
                }
                // check for correct parent
                if p == pid {
		    cpu += GetCPUUsage(p)
		    mem += GetMemoryUsage(p)
                }
	}
	cpuCounter := Counter{Name: "cpu", Value: cpu, Unit: "%"}
	memCounter := Counter{Name: "memory", Value: mem, Unit: "kb"}
	counters := make([]Counter, 2)
	counters[0] = cpuCounter
	counters[1] = memCounter
	// create metric
	metric = ContainerMetric{ContainerID: containerID, Counters: counters, Type: "container"}
	return metric, nil
}
