package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dotcloud/docker"
	"io"
	"io/ioutil"
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

const VERSION string = "0.1.1"

var (
	dockerPath    string
	shipyardURL   string
	shipyardKey   string
	runInterval   int
	registerAgent bool
	version       bool
	address       string
	port          int
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
		Command string
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
	flag.StringVar(&dockerPath, "docker", "/var/run/docker.sock", "Path to Docker socket")
	flag.StringVar(&shipyardURL, "url", "", "Shipyard URL")
	flag.StringVar(&shipyardKey, "key", "", "Shipyard Agent Key")
	flag.IntVar(&runInterval, "interval", 5, "Run interval")
	flag.BoolVar(&registerAgent, "register", false, "Register Agent with Shipyard")
	flag.BoolVar(&version, "version", false, "Shows Agent Version")
	flag.StringVar(&address, "address", "", "Listen address (default: 0.0.0.0)")
	flag.IntVar(&port, "port", 4500, "Agent Listen Port")

	flag.Parse()

	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
}

func newDockerClient() (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", dockerPath)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
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

func getContainers() []APIContainer {
	path := "/containers/json?all=1"
	c, err := newDockerClient()
	defer c.Close()
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	defer resp.Body.Close()
	var containers []APIContainer
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&containers); err != nil {
			log.Fatal(err)
		}
	}
	return containers
}

func inspectContainer(id string) *docker.Container {
	path := fmt.Sprintf("/containers/%s/json?all=1", id)
	c, err := newDockerClient()
	defer c.Close()
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
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
	path := fmt.Sprintf("%s/images/json?all=0", dockerPath)
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
		//go pushImages(jobs, pushGroup)
		pushGroup.Wait()
	}

	// wait for all request to finish processing before returning
	updaterGroup.Wait()
}

// Registers with Shipyard at the specified URL
func register() string {
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
	return data.Key
}

func dockerHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("Docker Request: %s", req.URL.Path)
	client, e := newDockerClient()
	path := req.URL.Path
	if strings.Index(path, "attach") != -1 {
		path = fmt.Sprintf("%s?logs=1&stream=0&stdout=1", path)
	}
	if e != nil {
		log.Printf("Error requesting %s from Docker: %s", path, e)
		return
	}
	req, err := http.NewRequest(req.Method, path, req.Body)
	if err != nil {
		log.Printf("Error requesting %s from Docker: %s", path, err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error requesting %s from Docker: %s", path, err)
		return
	}
	w.WriteHeader(resp.StatusCode)
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error parsing response from Docker: %s", err)
		return
	}
	w.Write([]byte(content))
}

type UnixHandler struct {
	path string
}

func (h *UnixHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := net.Dial("unix", h.path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error connecting to unix socket: %s", err)
		return
	}
	c := httputil.NewClientConn(conn, nil)
	defer c.Close()

	res, err := c.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error client.Do: %s", err)
		return
	}
	defer res.Body.Close()

	copyHeader(w.Header(), res.Header)
	if _, err := io.Copy(w, res.Body); err != nil {
		log.Printf("Error io.Copy body: %s", err)
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func createTcpHandler(e string) http.Handler {
	u, err := url.Parse(e)
	if err != nil {
		log.Fatal(err)
	}
	return httputil.NewSingleHostReverseProxy(u)
}

func createUnixHandler(e string) http.Handler {
	return &UnixHandler{e}
}

func createHandler(e string) http.Handler {
	var (
		mux = http.NewServeMux()
		h   http.Handler
	)

	if strings.Contains(e, "http") {
		h = createTcpHandler(e)
	} else {
		h = createUnixHandler(e)
	}

	mux.Handle("/", h)
	return mux
}

func main() {
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", runInterval))
	if err != nil {
		log.Fatal(err)
	}

	if shipyardURL == "" {
		fmt.Println("Error: You must specify a Shipyard URL")
		os.Exit(1)
	}

	if registerAgent && shipyardKey == "" {
		shipyardKey = register()
	}

	log.Printf("Shipyard Agent (%s)\n", shipyardURL)

	go listen(duration)

	handler := createHandler(dockerPath)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", address, port), handler))
}
