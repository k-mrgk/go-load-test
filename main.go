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
	"sync"
	"time"

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
	results         EachResult
	Transactions    int
	Availability    float64
	TransactionRate float64
	Throughput      float64
}

func main() {
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
			},
			Timeout: time.Duration(60) * time.Second,
		}

		ch1 := make(chan struct{})
		ch2 := make(chan EachResult)
		wg := new(sync.WaitGroup)
		t := time.Now()

		for i := 0; i < c.GlobalInt("concurrent"); i++ {
			wg.Add(1)
			go httpAccess(c, u, client, ch1, ch2, wg, t, i)
		}
		time.Sleep(time.Duration(c.GlobalInt("time")) * time.Second)
		close(ch1)

		var result AllResult
		result.results.Longest = 0
		result.results.Shortest = 999

		for i := 0; i < c.GlobalInt("concurrent"); i++ {
			res := <-ch2
			result.results.Data += res.Data
			result.results.Response += res.Response
			result.results.Success += res.Success
			result.results.Failed += res.Failed
			if result.results.Longest < res.Longest {
				result.results.Longest = res.Longest
			}
			if result.results.Shortest > res.Shortest {
				result.results.Shortest = res.Shortest
			}
		}
		wg.Wait()

		result.Transactions = result.results.Success + result.results.Failed
		if result.results.Failed != 0 {
			result.Availability = 100 - 100*float64(result.Transactions)/float64(result.results.Failed)
		} else {
			result.Availability = 100
		}
		result.TransactionRate = float64(result.Transactions) / float64(c.GlobalInt("time"))
		result.Throughput = float64(result.results.Data) / float64(c.GlobalInt("time"))
		result.results.Response /= float64(result.results.Success)

		fmt.Printf("Transactions: %21d hits\nAvailability: %21.2f %%\nData transferred: %17.2f MB\nResponse time: %20.2f secs\nTransaction rate: %17.2f trans/sec\nThroughput: %23.2f MB/sec\nSuccessful transactions: %10d\nFailed transactions: %14d\nLongest transaction: %14.2f\nShortest transaction: %13.2f\n",
			result.Transactions, result.Availability, float64(result.results.Data)/1024/1024, result.results.Response, result.TransactionRate, result.Throughput/1024/1024, result.results.Success, result.results.Failed, result.results.Longest, result.results.Shortest)

		return nil

	}
	app.Run(os.Args)

}

func httpAccess(c *cli.Context, u *url.URL, client *http.Client, receiveCh chan struct{}, sendCh chan EachResult, wg *sync.WaitGroup, t0 time.Time, number int) {

	var result EachResult
	result.Shortest = 999
	result.Longest = 0

	rand.Seed(time.Now().UnixNano() + int64(number))

Label:
	for {
		select {
		case <-receiveCh:
			//fmt.Println("Closed channnel ", number)
			break Label
		default:
			addrs, err := net.LookupHost(u.Host)
			if err != nil {
				log.Println(err)
				result.Failed++
				break
			}
			addr := addrs[0]

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

			if err != nil {
				log.Println(err)
				result.Failed++
				break
			}
			defer response.Body.Close()

			t2 := time.Now()
			tsub1 := t2.Sub(t1)
			tsub2 := t2.Sub(t0)
			restime := tsub1.Seconds()

			if !c.GlobalBool("quiet") {
				//log.Printf("%s %d %5.2f secs: %d bytes %s %s %d\n",
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
			time.Sleep(time.Duration(c.GlobalFloat64("delay")*rand.Float64()*1000) * time.Millisecond)
		}
	}

	sendCh <- result
	wg.Done()
}
