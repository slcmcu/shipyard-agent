package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
)

// Gets the CPU usage in percent for specified process id (pid)
func getCPUUsage(pid int) int {
	up, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		log.Fatal("Unable to read /proc/uptime: ", err)
	}
	stat, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		log.Fatal("Unable to read /proc: ", err)
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
func getMemUsage(pid int) int {
	stat, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/status", pid))
	if err != nil {
		log.Fatal("Unable to read /proc: ", err)
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
