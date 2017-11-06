package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/urfave/cli"
)

type EachResult struct {
	Data     int
	Response float64
	Success  int
	Failed   int
	Longest  float64
	Shortest float64
}

type AllResult struct {
	Results         EachResult
	Transactions    int
	Availability    float64
	TransactionRate float64
	Throughput      float64
}

//var ip map[string]int
var mu *sync.Mutex
var cpus int
var stopCh chan struct{}
var resCh chan EachResult

var dnsIP = [...]string{
	"192.168.11.83", "192.168.11.84", "192.168.11.85",
	"192.168.11.86", "192.168.11.87", "192.168.11.88",
	"192.168.11.89", "192.168.11.90", "192.168.11.91",
	"192.168.11.92", "192.168.11.93", "192.168.11.94",
	"192.168.11.95", "192.168.11.96", "192.168.11.97",
	"192.168.11.98", "192.168.11.99", "192.168.11.100",
	"192.168.11.160", "192.168.11.161", "192.168.11.162",
	"192.168.11.163", "192.168.11.164", "192.168.11.165",
}

func init() {
	cpus = runtime.NumCPU()
	runtime.GOMAXPROCS(cpus)
	if cpus == 1 {
		rand.Seed(time.Now().UnixNano())
	}
}

func main() {

	ip = make(map[string]int)
	mu = new(sync.Mutex)

	app := cli.NewApp()
	app.Name = "go-access"
	app.Usage = "HTTP Stress Test Tool for golang"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "benchmark, b",
			Usage: "Benchmark mode",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Quiet mode",
		},
		cli.IntFlag{
			Name:  "concurrent, c",
			Value: 10,
			Usage: "Number of simultaneous access",
		},
		cli.IntFlag{
			Name:  "time, t",
			Value: 60,
			Usage: "Time",
		},
		cli.Float64Flag{
			Name:  "delay, d",
			Value: 1.0,
			Usage: "Delay",
		},
	}

	//---- pre-processing
	app.Before = func(c *cli.Context) error {
		return nil
	}

	//---- post-processing
	app.After = func(c *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		if len(c.Args()) < 1 {
			return fmt.Errorf("Error: %s", "Not enough arguments")
		}
		u, err := url.Parse(c.Args().First())
		if err != nil {
			return err
		}

		client := &http.Client{
			Transport: &http.Transport{
				//Proxy: http.ProxyFromEnvironment,
				DisableKeepAlives: true,
				Dial: (&net.Dialer{
					Timeout: time.Duration(120) * time.Second,
					//KeepAlive: 30 * time.Second,
				}).Dial,
				//TLSHandshakeTimeout:   time.Duration(150) * time.Second,
				//ResponseHeaderTimeout: time.Duration(150) * time.Second,
				//ExpectContinueTimeout: time.Duration(150) * time.Second,
			},
		}

		m := &dns.Msg{
			MsgHdr: dns.MsgHdr{
				Authoritative:     false,
				AuthenticatedData: false,
				CheckingDisabled:  false,
				RecursionDesired:  true,
				Opcode:            dns.OpcodeQuery,
			},
			Question: make([]dns.Question, 1),
		}

		m.Rcode = dns.RcodeSuccess
		m.Question[0] = dns.Question{Name: dns.Fqdn(u.Host), Qtype: dns.TypeA, Qclass: dns.ClassINET}
		m.Id = dns.Id()

		stopCh = make(chan struct{})
		resCh = make(chan EachResult, c.GlobalInt("concurrent"))

		t := time.Now()
		fmt.Println(t.Format("2006-01-02 15:04:05"))

		for i := 0; i < c.GlobalInt("concurrent"); i++ {
			go httpAccess(c, u, client, t, i, len(dnsIP), m)
		}
		time.Sleep(time.Duration(c.GlobalInt("time")) * time.Second)
		close(stopCh)

		var result AllResult
		result.Results.Longest = 0
		result.Results.Shortest = 999

		for i := 0; i < c.GlobalInt("concurrent"); i++ {
			res := <-resCh
			result.Results.Data += res.Data
			result.Results.Response += res.Response
			result.Results.Success += res.Success
			result.Results.Failed += res.Failed
			if result.Results.Longest < res.Longest {
				result.Results.Longest = res.Longest
			}
			if result.Results.Shortest > res.Shortest {
				result.Results.Shortest = res.Shortest
			}
		}

		result.Transactions = result.Results.Success + result.Results.Failed
		if result.Results.Failed != 0 {
			result.Availability = 100 - 100*float64(result.Results.Failed)/float64(result.Transactions)
		} else {
			result.Availability = 100
		}
		result.TransactionRate = float64(result.Transactions) / float64(c.GlobalInt("time"))
		result.Throughput = float64(result.Results.Data) / float64(c.GlobalInt("time"))
		result.Results.Response /= float64(result.Results.Success)

		fmt.Printf("Transactions: %21d hits\nAvailability: %21.2f %%\nData transferred: %17.2f MB\nResponse time: %20.2f secs\nTransaction rate: %17.2f trans/sec\nThroughput: %23.2f MB/sec\nSuccessful transactions: %10d\nFailed transactions: %14d\nLongest transaction: %14.2f\nShortest transaction: %13.2f\n",
			result.Transactions, result.Availability, float64(result.Results.Data)/1024/1024, result.Results.Response, result.TransactionRate, result.Throughput/1024/1024, result.Results.Success, result.Results.Failed, result.Results.Longest, result.Results.Shortest)

		return nil

	}
	app.Run(os.Args)

}

func httpAccess(c *cli.Context, u *url.URL, client *http.Client, t0 time.Time, num, l int, m *dns.Msg) {

	var result EachResult
	var rnd *rand.Rand
	var q *dns.Msg
	var err error
	var r int

	result.Shortest = 999
	result.Longest = 0
	if cpus == 1 {
		rnd = rand.New(rand.NewSource(time.Now().UnixNano() + int64(num)))
	}
	d := new(dns.Client)
	d.Net = "udp4"

Label:
	for {
		select {
		case <-stopCh:
			break Label
		default:
			if cpus == 1 {
				r = rand.Intn(l)
			} else {
				r = rnd.Intn(l)
			}
			q, _, err = d.Exchange(m, dnsIP[r]+":53")
			if err != nil {
				log.Println(err)
				result.Failed++
				break
			}

			addr := (q.Answer[0].(*dns.A).A).String()

			/*
				mu.Lock()
				ip[addr]++
				mu.Unlock()
			*/

			url := u.Scheme + "://" + addr + u.Path
			request, err := http.NewRequest("GET", url, nil)
			if err != nil {
				log.Println(err)
				result.Failed++
				break
			}

			t1 := time.Now()
			response, err := client.Do(request)
			if err != nil {
				log.Println(err)
				result.Failed++
				break
			}

			body, err := ioutil.ReadAll(response.Body)
			flag := false
			func() {
				if err != nil {
					log.Println(err)
					result.Failed++
					flag = true
				}
				defer response.Body.Close()
			}()
			if flag {
				break
			}
			t2 := time.Now()
			tsub1 := t2.Sub(t1)
			tsub2 := t2.Sub(t0)
			restime := tsub1.Seconds()

			if !c.GlobalBool("quiet") {
				fmt.Printf("%s %d %5.2f secs: %d bytes %s %s %d\n",
					response.Proto, response.StatusCode, restime, len(body), u.Path, addr, int(tsub2.Seconds()))
			}
			result.Success++
			result.Data += len(body)
			result.Response += restime
			if result.Longest < restime {
				result.Longest = restime
			}
			if result.Shortest > restime {
				result.Shortest = restime
			}

		}
		if !c.GlobalBool("benchmark") {
			var fnum float64
			if cpus == 1 {
				fnum = rand.Float64()
			} else {
				fnum = rnd.Float64()
			}
			time.Sleep(time.Duration(c.GlobalFloat64("delay")*fnum*1000) * time.Millisecond)
		}
	}

	resCh <- result
}
