package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"syscall"

	"al.essio.dev/pkg/shellescape"
	"github.com/abenz1267/elephant/v2/internal/util"
	"github.com/abenz1267/elephant/v2/pkg/common"
	"github.com/abenz1267/elephant/v2/pkg/pb/pb"
)

var (
	Name       = "websearch"
	NamePretty = "Websearch"
	config     *Config
	engineMap  = make(map[string]int)
)

//go:embed README.md
var readme string

type Config struct {
	common.Config `koanf:",squash"`
	Engines       []Engine `koanf:"engines" desc:"entries" default:"google"`
	Command       string   `koanf:"command" desc:"default command to be executed. supports %VALUE%." default:"xdg-open"`
	DefaultScore  int32    `koanf:"default_score" desc:"score increment for default engines" default:"0"`
	MatchScore    int32    `koanf:"match_score" desc:"score increment for matched engines" default:"0"`
}

type Engine struct {
	Name      string `koanf:"name" desc:"name of the entry" default:""`
	Default   bool   `koanf:"default" desc:"entry to display when querying multiple providers" default:""`
	Alias     string `koanf:"alias" desc:"prefix to actively trigger this entry" default:""`
	URL       string `koanf:"url" desc:"url, example: 'http://google.com'" default:""`
	SearchURL string `koanf:"search_url" desc:"url, example: 'http://google.com/search?q=%TERM%'" default:""`
	Icon      string `koanf:"icon" desc:"icon to display, fallsback to global" default:""`
}

func Setup() {
	config = &Config{
		Config: common.Config{
			Icon:     "applications-internet",
			MinScore: 20,
		},
		Command:           "xdg-open",
	}

	common.LoadConfig(Name, config)

	if config.NamePretty != "" {
		NamePretty = config.NamePretty
	}

	if len(config.Engines) == 0 {
		config.Engines = append(config.Engines, Engine{
			Name:      "Google",
			Default:   true,
			URL:       "http://google.com",
			SearchURL: "http://google.com/search?q=%TERM%",
		})
	}

	if len(config.Engines) == 1 {
		config.Engines[0].Default = true
	}

	for i, eng := range config.Engines {
		if eng.Alias == "" {
			config.Engines[i].Alias = strings.ReplaceAll(strings.ToLower(eng.Name), " ", "-")
		}

		if _, ok := engineMap[eng.Name]; ok {
			slog.Error(Name, "Name collision", eng.Name)
			continue
		}

		engineMap[eng.Name] = i
	}
}

func Available() bool {
	return true
}

func PrintDoc() {
	fmt.Println(readme)
	fmt.Println()
	util.PrintConfig(Config{}, Name)
}

const (
	ActionSearch  = "s"
	ActionSearchPrefixed  = "sp"
	ActionOpen = "o"
)

func Activate(single bool, identifier string, action string, query string, args string, format uint8, conn net.Conn) {
	eng := config.Engines[engineMap[identifier]]
	finalUrl := ""

	switch action {
	case ActionOpen:
		finalUrl = eng.URL
	case ActionSearch:
		finalUrl = eng.SearchURL
	case ActionSearchPrefixed:
		_, query = splitAliasAndQuery(query)
		finalUrl = eng.SearchURL
	default:
		return
	}

	finalUrl = strings.ReplaceAll(finalUrl, "%TERM%", url.QueryEscape(query))

	open(finalUrl)
}

func open(url string) {
	cmd := exec.Command("sh", "-c", strings.TrimSpace(fmt.Sprintf("%s %s %s", common.LaunchPrefix(""), config.Command, shellescape.Quote(url))))

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	err := cmd.Start()
	if err != nil {
		slog.Error(Name, "activate", err)
	} else {
		go func() {
			cmd.Wait()
		}()
	}
}

func splitAliasAndQuery(query string) (string, string) {
	before, after, _ := strings.Cut(query, " ")
	return before, after
}

func entriesFor(engine Engine, query string) []*pb.QueryResponse_Item {
	entries := []*pb.QueryResponse_Item{}

	var score int32 = config.MinScore
	if engine.Default {
		score += config.DefaultScore
	}

	if engine.SearchURL != "" {
		entries = append(entries, &pb.QueryResponse_Item{
			Identifier: engine.Name,
			Text:       fmt.Sprintf("Search \"%s\" on %s", query, engine.Name),
			Actions:    []string{ActionSearch},
			Icon:       Icon(),
			Provider:   Name,
			Score:      score,
		})
	}

	alias, query := splitAliasAndQuery(query)

	if engine.Alias != alias {
		return entries
	}

	score += config.MatchScore

	if engine.URL != "" {
		entries = append(entries, &pb.QueryResponse_Item{
			Identifier: engine.Name,
			Text:       fmt.Sprintf("Open %s", engine.Name),
			Actions:    []string{ActionOpen},
			Icon:       Icon(),
			Provider:   Name,
			Score:      score,
		})
	}

	if query != "" && engine.SearchURL != "" {
		entries = append(entries, &pb.QueryResponse_Item{
			Identifier: engine.Name,
			Text:       fmt.Sprintf("Search \"%s\" on %s", query, engine.Name),
			Actions:    []string{ActionSearchPrefixed},
			Icon:       Icon(),
			Provider:   Name,
			Score:      score + 1,
		})
	}

	return entries
}

func Query(conn net.Conn, query string, _ bool, _ bool, _ uint8) []*pb.QueryResponse_Item {
	entries := []*pb.QueryResponse_Item{}

	for _, engInd := range engineMap {
		eng := config.Engines[engInd]
		entries = append(entries, entriesFor(eng, query)...)
	}

	return entries
}

func Icon() string {
	return config.Icon
}

func HideFromProviderlist() bool {
	return config.HideFromProviderlist
}

func State(provider string) *pb.ProviderStateResponse {
	return &pb.ProviderStateResponse{}
}
