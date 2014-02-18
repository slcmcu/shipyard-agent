package utils

import (
        "bytes"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"strconv"
	"strings"
        "os/exec"
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
	var (
		utime, _     = strconv.Atoi(parts[14])
		stime, _     = strconv.Atoi(parts[15])
		cutime, _    = strconv.Atoi(parts[16])
		cstime, _    = strconv.Atoi(parts[17])
		startTime, _ = strconv.Atoi(parts[22])
		totalTime    = utime + stime + cutime + cstime
		uptime, _    = strconv.Atoi(string(up))
		seconds      = uptime - (startTime / 1000)
		cpuUsage     = 100 * ((totalTime / 1000) / seconds)
	)
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

// Check all running processes for docker containers and create
// metrics for each
func GetContainerMetrics() []ContainerMetric {
	dirs, err := ioutil.ReadDir("/proc")
	if err != nil {
		log.Println(err)
	}
	var metrics []ContainerMetric
	for _, dir := range dirs {
		// get all process dirs
		pidDir := path.Join("/proc", dir.Name())
		isDir, _ := IsDir(pidDir)
		// filter out "self" (current command)
		if isDir && dir.Name() != "self" {
			// make sure process has a cmdline file
			cmdlinePath := path.Join(pidDir, "cmdline")
			exists, _ := Exists(cmdlinePath)
			if exists {
				// read cmdline of process
				cmdline, err := ioutil.ReadFile(cmdlinePath)
				if err != nil {
					log.Println(err)
				}
				// convert string pid to int
				// check for lxc-start command
				if strings.Index(string(cmdline), "lxc-start") == 0 {
					// hack: split on -n and -f to get container id
					cmdlineParts := strings.Split(string(cmdline), "-n")
					sID := strings.Split(cmdlineParts[1], "-f")
					// remove \x00 characters from ID
					containerID := strings.Trim(sID[0], "\x00")
                                        // find all child process for container
                                        c := exec.Command("lxc-ps", "-n", containerID, "--", "o", "pid")
                                        var cOut bytes.Buffer
                                        c.Stdout = &cOut
                                        err = c.Run()
                                        if err != nil {
                                            log.Printf("Erroring getting metrics for %s: %s", containerID, err)
                                            continue
                                        }
                                        cpu := 0
                                        mem := 0
                                        for _, s := range strings.Split(cOut.String(), "\n") {
                                            if strings.Index(s, "CONTAINER") == -1 && s != "" {
                                                f := strings.Fields(s)
                                                pid, _ := strconv.Atoi(f[1])
                                                cpu += GetCPUUsage(pid)
                                                mem += GetMemoryUsage(pid)
                                            }
                                        }
					cpuCounter := Counter{Name: "cpu", Value: cpu, Unit: "%"}
					memCounter := Counter{Name: "memory", Value: mem, Unit: "kb"}
					counters := make([]Counter, 2)
					counters[0] = cpuCounter
					counters[1] = memCounter
					// create metric
					metric := ContainerMetric{ContainerID: containerID, Counters: counters, Type: "container"}
					metrics = append(metrics, metric)
				}
			}
		}
	}
	return metrics
}
