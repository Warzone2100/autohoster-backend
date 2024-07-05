package ispcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/maxsupermanhd/lac/v2"
)

type ISPChecker struct {
	cfg   lac.Conf
	hcl   *http.Client
	l     sync.Mutex
	cache map[string]LookupResponse
}

func NewISPChecker(cfg lac.Conf) *ISPChecker {
	c, err := loadCache(cfgGetCachePath(cfg))
	if err != nil {
		log.Printf("Failed to load ISP cache: %s", err.Error())
		c = map[string]LookupResponse{}
	}
	return &ISPChecker{
		cfg: cfg,
		hcl: &http.Client{
			Timeout: cfgGetTimeoutSeconds(cfg),
		},
		cache: c,
	}
}

func cfgGetTimeoutSeconds(cfg lac.Conf) time.Duration {
	return time.Duration(cfg.GetDSInt(2, "httpTimeoutSeconds")) * time.Second
}

func cfgGetCachePath(cfg lac.Conf) string {
	return cfg.GetDSString("ISPcache.json", "cachePath")
}

func cfgGetCreatePerms(cfg lac.Conf) os.FileMode {
	return fs.FileMode(cfg.GetDInt(644, "filePerms"))
}

func cfgGetUrlFmt(cfg lac.Conf) string {
	return cfg.GetDString("http://ip-api.com/json/%s?fields=21220864", "urlFmt")
}

type LookupResponse struct {
	IsProxy bool
	ASN     string
}

func (ch *ISPChecker) Lookup(ip string) (*LookupResponse, error) {
	ch.l.Lock()
	defer ch.l.Unlock()

	rspC, ok := ch.cache[ip]
	if ok {
		return &rspC, nil
	}

	rsp, err := ch.lookup(ip)
	if err != nil {
		return nil, err
	}
	if rsp != nil {
		ch.cache[ip] = *rsp
		ch.saveCache()
	}

	return rsp, err
}

func (ch *ISPChecker) lookup(ip string) (*LookupResponse, error) {
	url := fmt.Sprintf(cfgGetUrlFmt(ch.cfg), ip)
	r, err := ch.hcl.Get(url)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var rs struct {
		Status  string `json:"status"`
		Isp     string `json:"isp"`
		Org     string `json:"org"`
		As      string `json:"as"`
		Asname  string `json:"asname"`
		Mobile  bool   `json:"mobile"`
		Proxy   bool   `json:"proxy"`
		Hosting bool   `json:"hosting"`
	}
	err = json.Unmarshal(b, &rs)
	if err != nil {
		return nil, err
	}
	if rs.Status != "success" {
		return nil, fmt.Errorf("request to ip api failed: status %s (%s)", rs.Status, string(b))
	}
	return &LookupResponse{
		IsProxy: rs.Proxy,
		ASN:     rs.Asname,
	}, nil
}

func loadCache(path string) (map[string]LookupResponse, error) {
	ret := map[string]LookupResponse{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ret, nil
		}
		return nil, err
	}
	err = json.Unmarshal(b, &ret)
	return ret, err
}

func (ch *ISPChecker) saveCache() error {
	b, err := json.Marshal(ch.cache)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgGetCachePath(ch.cfg), b, cfgGetCreatePerms(ch.cfg))
}
