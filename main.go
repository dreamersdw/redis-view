package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/fzzy/radix/redis"
	"github.com/mgutz/ansi"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	version = "0.1"
	usage   = `Usage:
	redis-view [--url=URL] [--sep=SEP] [--only-keys] [--nowrap] [PATTERN...]
	redis-view --version
	redis-view --help

Example:
	redis-view 'tasks:*' 'metrics:*' `
)

var (
	redisClient *redis.Client
	wrap        bool
	turnOnColor bool
	redisURL    = "redis://127.0.0.1:6379"
	patterns    = []string{"*"}
	keySep      = ":"
	onlyKeys    = false
)

type treeNode struct {
	value    string
	children map[string]treeNode
}

func getConn() *redis.Client {
	if redisClient == nil {
		URL, err := url.Parse(redisURL)
		if err != nil {
			fmt.Printf("fail to parse url '%s'\n", redisURL)
			os.Exit(1)
		}

		address := URL.Host
		if !strings.Contains(address, ":") {
			address = fmt.Sprintf("%s:%d", URL.Host, 6379)
		}

		client, err := redis.Dial("tcp", address)
		if err != nil {
			fmt.Printf("unable connect to redis server\n")
			os.Exit(1)
		}

		redisClient = client
	}
	return redisClient
}

func populate(tree *treeNode, keys []string, sep string) {
	for _, key := range keys {
		var node = *tree
		for _, part := range strings.Split(key, sep) {
			_, ok := node.children[part]
			if !ok {
				node.children[part] = treeNode{
					value:    part,
					children: make(map[string]treeNode)}
			}
			node = node.children[part]
		}
	}
}

func mapKeys(m map[string]treeNode) []string {
	var keys = make([]string, len(m))[0:0]
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func query(key string) (rtype string, ttl int64, val interface{}) {
	r := getConn()

	rtype, _ = r.Cmd("type", key).Str()
	ttl, _ = r.Cmd("ttl", key).Int64()

	if onlyKeys {
		val = ""
		return
	}

	switch rtype {
	case "string":
		val, _ = r.Cmd("get", key).Str()
	case "list":
		val, _ = r.Cmd("lrange", key, 0, -1).List()
	case "set":
		val, _ = r.Cmd("smembers", key).List()
	case "hash":
		val, _ = r.Cmd("hgetall", key).Hash()
	case "zset":
		val, _ = r.Cmd("zrange", key, 0, -1, "WITHSCORES").Hash()
	}
	return
}

func isBinary(bytes []byte) bool {
	if len(bytes) == 0 {
		return false
	}

	invisible := 0
	for _, b := range bytes {
		if (32 <= b && b < 127) || b == '\n' || b == '\t' || b == 'r' || b == 'f' || b == 'b' {
		} else {
			invisible++
		}
	}

	if float64(invisible)/float64(len(bytes)) >= 0.3 {
		return true
	}
	return false
}

func bitset(bytes []byte) []byte {
	seq := make([]byte, 8*len(bytes))
	for index, char := range bytes {
		for i := 0; i < 8; i++ {
			bit := (char >> uint(i)) & 0x1
			if bit == 0 {
				seq[index*8+(7-i)] = '0'
			} else {
				seq[index*8+(7-i)] = '1'
			}
		}
	}
	return seq
}

func prettyPrint(val interface{}, prefix string, wrap bool, isLast bool) string {
	var result []byte
	var newPrefix = prefix
	if !isLast {
		newPrefix = prefix + "|"
	}
	switch val.(type) {
	case map[string]string:
		if !wrap || len(val.(map[string]string)) <= 1 {
			result, _ = json.Marshal(val)
		} else {
			result, _ = json.MarshalIndent(val, newPrefix, "   ")
		}
	case []string:
		if !wrap || len(val.([]string)) <= 1 {
			result, _ = json.Marshal(val)
		} else {
			result, _ = json.MarshalIndent(val, newPrefix, "   ")
		}
	case string:
		result = []byte(val.(string))
		if isBinary(result) {
			result = bitset(result)
		}
	}
	return string(result)
}

func colorize(s string, style string) string {
	if turnOnColor {
		return ansi.Color(s, style)
	}
	return s
}

func plotNode(node treeNode, key string, leading string, isLast bool) {
	var sep string
	if isLast {
		sep = "└── "
	} else {
		sep = "├── "
	}

	var extra string
	if len(node.children) == 0 {
		rtype, ttl, val := query(key)

		var sttl = ""
		if ttl != -1 {
			sttl = strconv.Itoa(int(ttl))
		}

		extra = fmt.Sprintf("%s %s %s %s", "#",
			colorize(rtype, "yellow"),
			colorize(sttl, "red"),
			prettyPrint(val, leading, wrap, isLast))
	}

	nodeVal := colorize(node.value, "blue")

	fmt.Printf("%s%s%s %s\n", leading, sep, nodeVal, extra)
}

func plot(node treeNode, key string, leading string) {
	parts := mapKeys(node.children)
	for index, part := range parts {
		var newKey = ""
		if key == "" {
			newKey = part
		} else {
			newKey = key + ":" + part
		}
		isLast := index == len(parts)-1
		plotNode(node.children[part], newKey, leading, isLast)
		var newLeading string
		if isLast {
			newLeading = leading + "    "
		} else {
			newLeading = leading + "│   "
		}
		plot(node.children[part], newKey, newLeading)
	}
}

func main() {
	opt, err := docopt.Parse(usage, nil, false, "", false, false)
	if err != nil {
		os.Exit(1)
	}

	if opt["--version"] != false {
		fmt.Println(version)
		return
	}

	if opt["--help"] != false {
		fmt.Println(usage)
		return
	}

	wrap = !opt["--nowrap"].(bool)

	onlyKeys = opt["--only-keys"].(bool)

	turnOnColor = terminal.IsTerminal(int(os.Stdout.Fd()))

	if opt["--sep"] != nil {
		keySep = opt["--sep"].(string)
	}

	if opt["--url"] != nil {
		redisURL = opt["--url"].(string)
	}

	if ps := opt["PATTERN"].([]string); len(ps) != 0 {
		patterns = ps
	}

	r := getConn()

	tree := &treeNode{value: "/", children: make(map[string]treeNode)}
	for _, pattern := range patterns {
		keys, err := r.Cmd("KEYS", pattern).List()
		if err != nil {
			continue
		}
		populate(tree, keys, keySep)
	}

	plot(*tree, "", "")
}
