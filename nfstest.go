package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"
	"time"
)

var (
	nfsDir          string // nfs mount path
	nfsFile         string // path of file to read
	stoppWorkload   chan bool
	rateCounter     chan Counters
	metricMutex     sync.Mutex
	workloadRunning bool

	// CurrentMetrics latest metrics collected
	CurrentMetrics Metrics
)

// Metrics return in api request
type Metrics struct {
	ReadAVG float64 `json:"avg-read-ms"`
	ReadMax float64 `json:"max-read-ms"`
	ReadMin float64 `json:"min-read-ms"`
	Rate    float64 `json:"rate-second"`
}

// Update current metrics
func (m *Metrics) Update(ra, rm, rn, r float64) {
	metricMutex.Lock()
	m.ReadAVG = ra
	m.ReadMax = rm
	m.ReadMin = rn
	m.Rate = r
	metricMutex.Unlock()
}

// GetCurrent metrics
func (m *Metrics) GetCurrent() Metrics {
	metricMutex.Lock()
	newM := *m
	metricMutex.Unlock()
	return newM
}

// Counters collect in each read operations
type Counters struct {
	Read float64
	Tick float64
}

func parseVCAP() {

	type NFSMounts struct {
		Dir string `json:"container_dir"`
	}
	type NFSVMounts struct {
		VolumneMounts []NFSMounts `json:"volume_mounts"`
	}
	type VCAP struct {
		NFS []NFSVMounts `json:"nfs"`
	}

	service := new(VCAP)
	env := os.Getenv("VCAP_SERVICES")
	if env == "" {
		panic("VCAP_SERVICES NOT DEFINED")
	}
	err := json.Unmarshal([]byte(env), &service)
	if err != nil {
		panic("UNABLE TO PARSE VCAP_SERVICES")
	}
	if len(service.NFS) <= 0 || len(service.NFS[0].VolumneMounts) <= 0 {
		panic("NO VOLUME MOUNTS FOUND")
	}
	if service.NFS[0].VolumneMounts[0].Dir == "" {
		panic("VOLUME MOUNT IS EMPTY")
	}
	nfsDir = service.NFS[0].VolumneMounts[0].Dir
	fmt.Printf("NFS Share set to %s\n", nfsDir)
}

func rateMeter(ticks chan Counters) {
	c := Metrics{0, 0, 0, 0}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticks:
			c.ReadAVG += t.Read
			c.Rate += t.Tick
			if c.ReadMax < t.Read || c.ReadMax == 0 {
				c.ReadMax = t.Read
			}
			if c.ReadMin > t.Read || c.ReadMin == 0 {
				c.ReadMin = t.Read
			}
		case <-ticker.C:
			avg := 0.0
			if c.ReadAVG > 0 {
				avg = c.ReadAVG / c.Rate
			}
			CurrentMetrics.Update(avg, c.ReadMax, c.ReadMin, c.Rate)
			c = Metrics{0, 0, 0, 0}
		}
	}
}

func workloadGen(interval time.Duration, stopper chan bool, ticks chan Counters) {
	workloadRunning = true
	for {
		select {
		case stop := <-stopper:
			if stop {
				return
			}
		default:
			now := time.Now()
			_, err := ioutil.ReadFile(path.Join(nfsDir, nfsFile))
			if err != nil {
				fmt.Println(err)
			}
			ticks <- Counters{float64(time.Now().Nanosecond()-now.Nanosecond()) / 1000000, 1}
			time.Sleep(interval)
		}
	}
}

func apiStop(w http.ResponseWriter, r *http.Request) {
	if workloadRunning {
		stoppWorkload <- true
		workloadRunning = false
	}
	w.Write([]byte("success\n"))
}

func apiRun(w http.ResponseWriter, r *http.Request) {
	if workloadRunning {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Workload already running\n"))
		return
	}
	params := r.URL.Query()
	interval := 1000 * time.Millisecond
	if params.Get("interval") != "" {
		i, err := strconv.Atoi(params.Get("interval"))
		if err != nil {
			fmt.Println(err)
		} else {
			interval = time.Duration(i) * time.Millisecond
		}
	}
	go workloadGen(interval, stoppWorkload, rateCounter)
	w.Write([]byte("success\n"))
}

func apiMetrics(w http.ResponseWriter, r *http.Request) {
	m := CurrentMetrics.GetCurrent()
	fmt.Printf("%v\n", m)
	jdata, err := json.Marshal(m)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Failed to Marshal data: %s\n", err)))
	}
	w.Write(append(jdata, byte('\n')))
}

func main() {
	parseVCAP()
	stoppWorkload = make(chan bool, 0)
	rateCounter = make(chan Counters, 1000)
	metricMutex = sync.Mutex{}
	CurrentMetrics = Metrics{0, 0, 0, 0}

	nfsFile = os.Getenv("FILENAME")

	go rateMeter(rateCounter)

	http.HandleFunc("/api/stop", apiStop)
	http.HandleFunc("/api/run", apiRun)
	http.HandleFunc("/api/metrics", apiMetrics)
	http.Handle("/img/", http.FileServer(http.Dir(nfsDir)))
	err := http.ListenAndServe(":"+os.Getenv("PORT"), nil)
	if err != nil {
		panic(err)
	}
}
