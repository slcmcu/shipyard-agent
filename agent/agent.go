package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dotcloud/docker"
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

const VERSION string = "0.0.9"

var (
	dockerURL     string
	shipyardURL   string
	shipyardKey   string
	runInterval   int
	registerAgent bool
	version       bool
	port          int
)

type (
	AgentData struct {
		Key string `json:"key"`
	}

	ContainerData struct {
		Container docker.APIContainers
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
	flag.IntVar(&runInterval, "interval", 5, "Run interval")
	flag.BoolVar(&registerAgent, "register", false, "Register Agent with Shipyard")
	flag.BoolVar(&version, "version", false, "Shows Agent Version")
	flag.IntVar(&port, "port", 4500, "Agent Listen Port")

	flag.Parse()

	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	if shipyardURL == "" {
		fmt.Println("Error: You must specify a Shipyard URL")
		os.Exit(1)
	}
}

func updater(jobs <-chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	client := &http.Client{}

	for obj := range jobs {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
			log.Println(err)
			continue
		}
		s := []string{shipyardURL, obj.Path}
		req, err := http.NewRequest("POST", strings.Join(s, ""), buf)
		if err != nil {
			log.Println(err)
			continue
		}

		req.Header.Set("Authorization", fmt.Sprintf("AgentKey:%s", shipyardKey))
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err)
			continue
		}
		defer resp.Body.Close()
	}
}

func getContainers() []*docker.APIContainers {
	path := fmt.Sprintf("%s/containers/json?all=1", dockerURL)
	resp, err := http.Get(path)
	defer resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	var containers []*docker.APIContainers
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&containers); err != nil {
			log.Fatal(err)
		}
	}
	return containers
}

func inspectContainer(id string) *docker.Container {
	path := fmt.Sprintf("%s/containers/%s/json?all=1", dockerURL, id)
	resp, err := http.Get(path)
	defer resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	var container *docker.Container
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&container); err != nil {
			log.Fatal(err)
		}
	}
	return container
}

func getImages() []*Image {
	path := fmt.Sprintf("%s/images/json?all=0", dockerURL)
	resp, err := http.Get(path)
	defer resp.Body.Close()
	if err != nil {
		log.Fatal(err)
	}
	var images []*Image
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&images); err != nil {
			log.Fatal(err)
		}
	}
	return images
}

func pushContainers(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	containers := getContainers()
	data := make([]ContainerData, len(containers))
	for x, c := range containers {
		i := inspectContainer(c.ID)
		containerData := ContainerData{Container: *c, Meta: i}
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

func listen(d time.Duration) {
	var (
		updaterGroup = &sync.WaitGroup{}
		pushGroup    = &sync.WaitGroup{}
		// create chan with a 2 buffer, we use a 2 buffer to sync the go routines so that
		// no more than two messages are being send to the server at one time
		jobs = make(chan *Job, 2)
	)

	go updater(jobs, updaterGroup)

	for _ = range time.Tick(d) {
		go pushContainers(jobs, pushGroup)
		go pushImages(jobs, pushGroup)
		pushGroup.Wait()
	}

	// wait for all request to finish processing before returning
	updaterGroup.Wait()
}

// Registers with Shipyard at the specified URL
func register() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal(err)
	}
	blockedIPs := map[string]bool{
		"127.0.0.1":   false,
		"172.17.42.1": false,
	}
	var hostIP string
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			log.Fatal(err)
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
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatal(err)
	}
	log.Println("Agent Key: ", data.Key)
}

func main() {
	if registerAgent {
		register()
		return
	}

	duration, err := time.ParseDuration(fmt.Sprintf("%ds", runInterval))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Shipyard Agent (%s)\n", shipyardURL)
	u, err := url.Parse(dockerURL)
	if err != nil {
		log.Fatal(err)
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

	go listen(duration)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), proxy); err != nil {
		log.Fatal(err)
	}
}
