package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/shiyanhui/dht"
	"net/http"
	_ "net/http/pprof"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"os"
)

type file struct {
	Path   []interface{} `json:"path"`
	Length int           `json:"length"`
}

type bitTorrent struct {
	InfoHash string `json:"infohash"`
	Name     string `json:"name"`
	Files    []file `json:"files,omitempty"`
	Length   int    `json:"length,omitempty"`
}

func main() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": "kafka-server:9092"})
	if err != nil {
		panic(err.Error())
	}
	address := os.Args[1]

	go func() {
		for e := range p.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					fmt.Printf("Delivery failed: %v\n", ev.TopicPartition)
				} else {
					fmt.Printf("Delivered message to %v\n", ev.TopicPartition)
				}
			}
		}
	}()

	w := dht.NewWire(65536, 1024, 256)
	go func() {
		for resp := range w.Response() {
			metadata, err := dht.Decode(resp.MetadataInfo)
			if err != nil {
				continue
			}
			info := metadata.(map[string]interface{})

			if _, ok := info["name"]; !ok {
				continue
			}

			bt := bitTorrent{
				InfoHash: hex.EncodeToString(resp.InfoHash),
				Name:     info["name"].(string),
			}

			if v, ok := info["files"]; ok {
				files := v.([]interface{})
				bt.Files = make([]file, len(files))

				for i, item := range files {
					f := item.(map[string]interface{})
					bt.Files[i] = file{
						Path:   f["path"].([]interface{}),
						Length: f["length"].(int),
					}
				}
			} else if _, ok := info["length"]; ok {
				bt.Length = info["length"].(int)
			}

			data, err := json.Marshal(bt)
			if err == nil {
				fmt.Printf("%s\n\n", data)
				sendData(data, p)
			}
		}
	}()
	go w.Run()

	config := dht.NewCrawlConfig()
	config.MaxNodes = 10000
	config.Address = address
	config.OnAnnouncePeer = func(infoHash, ip string, port int) {
		w.Request([]byte(infoHash), ip, port)
	}
	d := dht.New(config)

	d.Run()
}

func sendData(data []byte, p *kafka.Producer) () {
	topic := "dht_infohash"
	p.Produce(&kafka.Message{
		TopicPartition:kafka.TopicPartition{Topic:&topic, Partition: kafka.PartitionAny},
		Value: data,
	}, nil)
	p.Flush(1000)
}
