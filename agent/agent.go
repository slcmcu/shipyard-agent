package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dotcloud/docker"
	"github.com/shipyard/shipyard-agent/utils"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const VERSION string = "0.2.3"

var (
	dockerURL      string
	shipyardURL    string
	shipyardKey    string
	runInterval    int
	metricInterval int
	registerAgent  bool
	version        bool
	address        string
	port           int
)

type (
	AgentData struct {
		Key string `json:"key"`
	}
	Port struct {
		IP          string
		PrivatePort int
		PublicPort  int
		Type        string
	}

	APIContainer struct {
		Id      string
		Created int
		Image   string
		Status  string
		Command string `json:"string"`
		Ports   []Port
		Names   []string
	}

	ContainerData struct {
		Container APIContainer
		Meta      *docker.Container
	}

	Job struct {
		Path string
		Data interface{}
	}

	Image struct {
		Id          string
		Created     int
		RepoTags    []string
		Size        int
		VirtualSize int
	}
)

func init() {
	flag.StringVar(&dockerURL, "docker", "http://127.0.0.1:4243", "URL to Docker")
	flag.StringVar(&shipyardURL, "url", "", "Shipyard URL")
	flag.StringVar(&shipyardKey, "key", "", "Shipyard Agent Key")
	flag.IntVar(&runInterval, "interval", 5, "Run interval (seconds)")
	flag.IntVar(&metricInterval, "metric-interval", 60, "Metric interval (seconds)")
	flag.BoolVar(&registerAgent, "register", false, "Register Agent with Shipyard")
	flag.BoolVar(&version, "version", false, "Shows Agent Version")
	flag.StringVar(&address, "address", "", "Agent Listen Address (default: 0.0.0.0)")
	flag.IntVar(&port, "port", 4500, "Agent Listen Port")

	flag.Parse()

	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
}

func updater(jobs <-chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	client := &http.Client{}

	for obj := range jobs {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
			log.Printf("Error decoding JSON: %s", err)
			continue
		}
		s := []string{shipyardURL, obj.Path}
		req, err := http.NewRequest("POST", strings.Join(s, ""), buf)
		if err != nil {
			log.Printf("Error sending to Shipyard: %s", err)
			continue
		}

		req.Header.Set("Authorization", fmt.Sprintf("AgentKey:%s", shipyardKey))
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error sending to Shipyard: %s", err)
			continue
		}
		resp.Body.Close()
	}
}

func getContainers() []APIContainer {
	path := fmt.Sprintf("%s/containers/json?all=1", dockerURL)
	resp, err := http.Get(path)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	var containers []APIContainer
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&containers); err != nil {
			log.Fatalf("Error parsing container JSON from Docker: %s", err)
		}
	}
	resp.Body.Close()
	return containers
}

func inspectContainer(id string) *docker.Container {
	path := fmt.Sprintf("%s/containers/%s/json?all=1", dockerURL, id)
	resp, err := http.Get(path)
	if err != nil {
		log.Fatalf("Error inspecting container %s from Docker: %s", id, err)
	}
	var container *docker.Container
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&container); err != nil {
			log.Fatalf("Error parsing container JSON: %s", err)
		}
	}
	resp.Body.Close()
	return container
}

func getImages() []*Image {
	path := fmt.Sprintf("%s/images/json?all=0", dockerURL)
	resp, err := http.Get(path)
	if err != nil {
		log.Fatalf("Error requesting images from Docker: %s", err)
	}
	var images []*Image
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&images); err != nil {
			log.Fatalf("Error parsing image JSON: %s", err)
		}
	}
	resp.Body.Close()
	return images
}

func getContainerMetrics() []utils.ContainerMetric {
	containerMetrics := utils.GetContainerMetrics()
	return containerMetrics
}

func pushContainers(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	containers := getContainers()
	data := make([]ContainerData, len(containers))
	for x, c := range containers {
		i := inspectContainer(c.Id)
		containerData := ContainerData{Container: c, Meta: i}
		data[x] = containerData
	}

	jobs <- &Job{
		Path: "/agent/containers/",
		Data: data,
	}
}

func pushImages(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	images := getImages()
	jobs <- &Job{
		Path: "/agent/images/",
		Data: images,
	}
}

func pushContainerMetrics(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	metrics := getContainerMetrics()
	jobs <- &Job{
		Path: "/agent/metrics/",
		Data: metrics,
	}
}

func syncDocker(d time.Duration) {
	var (
		updaterGroup = &sync.WaitGroup{}
		pushGroup    = &sync.WaitGroup{}
		jobs         = make(chan *Job, 2)
	)

	go updater(jobs, updaterGroup)

	for _ = range time.Tick(d) {
		go pushContainers(jobs, pushGroup)
		go pushImages(jobs, pushGroup)
		pushGroup.Wait()
	}
	updaterGroup.Wait()

}

func syncMetrics(d time.Duration) {
	var (
		updaterGroup = &sync.WaitGroup{}
		metricGroup  = &sync.WaitGroup{}
		jobs         = make(chan *Job, 1)
	)
	go updater(jobs, updaterGroup)
	for _ = range time.Tick(d) {
		go pushContainerMetrics(jobs, metricGroup)
		metricGroup.Wait()
	}
	updaterGroup.Wait()
}

// Registers with Shipyard at the specified URL
func register() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Error registering with Shipyard: %s", err)
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("Error finding network interface addresses: %s", err)
	}
	blockedIPs := map[string]bool{
		"127.0.0.1":   false,
		"172.17.42.1": false,
	}
	var hostIP string
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			log.Fatalf("Error parsing CIDR from network address: %s", err)
		}
		// filter loopback
		if !ip.IsLoopback() {
			_, blocked := blockedIPs[string(ip)]
			if !blocked {
				hostIP = ip.String()
				break
			}
		}
	}

	var (
		vals = url.Values{"name": {hostname}, "port": {strconv.Itoa(port)}, "hostname": {hostIP}}
		data AgentData
	)
	log.Printf("Using %s for the Docker Host IP for Shipyard\n", hostIP)
	log.Println("If this is not correct or you want to use a different IP, please update the host in Shipyard")
	log.Printf("Registering at %s\n", shipyardURL)

	rURL := fmt.Sprintf("%v/agent/register/", shipyardURL)
	resp, err := http.PostForm(rURL, vals)
	if err != nil {
		log.Fatalf("Error registering with Shipyard: %s", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatalf("Error parsing JSON from Shipyard register: %s", err)
	}
	log.Println("Agent Key: ", data.Key)
	return data.Key
}

func main() {
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", runInterval))
	if err != nil {
		log.Fatal("Error parsing duration: %s", err)
	}
	metricDuration, err := time.ParseDuration(fmt.Sprintf("%ds", metricInterval))
	if err != nil {
		log.Fatal("Error parsing duration: %s", err)
	}

	if shipyardURL == "" {
		fmt.Println("Error: You must specify a Shipyard URL")
		os.Exit(1)
	}

	if registerAgent {
		register()
		os.Exit(0)
	}

	log.Printf("Shipyard Agent (%s)\n", shipyardURL)
	log.Printf("Listening on %s:%d", address, port)
	u, err := url.Parse(dockerURL)
	if err != nil {
		log.Fatalf("Error connecting to Docker (is Docker listening on TCP?): %s", err)
	}

	var (
		proxy    = httputil.NewSingleHostReverseProxy(u)
		director = proxy.Director
	)

	proxy.Director = func(req *http.Request) {
		src := strings.Split(req.RemoteAddr, ":")[0]
		log.Printf("Request from %s: %s\n", src, req.URL.Path)
		director(req)
	}

	go syncDocker(duration)
	go syncMetrics(metricDuration)

	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", address, port), proxy); err != nil {
		log.Fatalf("Error listening on port %d: %s", port, err)
	}
}
