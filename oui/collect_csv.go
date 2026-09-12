package oui

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gookit/gcli/v3/progress"
	"github.com/thatmattlove/go-macaddr"
)

const (
	workingUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.79 Safari/537.36"
)

func fetchCSV(client *http.Client, registry *Registry) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, registry.URL().String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", workingUserAgent)
	res, err := client.Do(req)
	if err != nil {
		if os.IsTimeout(err) {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		return nil, err
	}
	if res.StatusCode != 200 {
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("[%s] failed to download data from %s error: [%s] %v", registry.Name, registry.URL(), res.Status, string(b))
	}
	return res.Body, nil
}

func DownloadCSV(registry *Registry) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	body, err := fetchCSV(client, registry)
	if err != nil {
		return "", err
	}
	defer body.Close()
	file, err := os.CreateTemp("", registry.TempFilePattern())
	if err != nil {
		return "", err
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	if err != nil {
		return "", err
	}
	return file.Name(), nil
}

func readCSVRows(registry *Registry, r io.Reader, logger LoggerType) ([]*VendorDef, error) {
	results := make([]*VendorDef, 0)
	reader := csv.NewReader(r)
	reader.LazyQuotes = true
	var place int64
	for {
		var row []string
		row, err := reader.Read()
		if err == io.EOF {
			// Exit loop when file is done being read.
			if logger != nil {
				logger.Success("finished parsing vendors from %s registry", registry.Name)
			}
			break
		} else if err != nil {
			if logger != nil {
				logger.Err(err, "failed to read file '%s'", registry.FileName())
			}
		}
		if place == 0 {
			// Ignore header row.
			place++
			continue
		}
		place++
		if len(row) < 3 {
			// Ignore rows that don't conform to expected structure.
			if logger != nil {
				logger.Warn("skipping row %s", row)
			}
			continue
		}
		assignment := strings.TrimSpace(row[1])
		if !strings.Contains(assignment, "/") {
			assignment += fmt.Sprintf("/%d", registry.DefaultPrefixLen)
		}
		organization := row[2]
		org := strings.TrimSpace(organization)
		base, mp, err := macaddr.ParseMACPrefix(assignment)
		if err != nil {
			if logger != nil {
				logger.Err(err, "failed to parse OUI assignment")
			}
			continue
		}
		prefixLen := mp.PrefixLen()
		prefix := fmt.Sprintf("%s/%d", base.String(), prefixLen)
		v := &VendorDef{
			Org:      org,
			Length:   prefixLen,
			Prefix:   prefix,
			Registry: registry.Name,
		}
		results = append(results, v)
	}
	return results, nil
}

func ReadCSV(registry *Registry, fileName string, logger LoggerType) ([]*VendorDef, error) {
	file, err := os.Open(fileName)
	if err != nil {
		if logger != nil {
			logger.Err(err)
		}
		return nil, err
	}
	defer file.Close()
	return readCSVRows(registry, file, logger)
}

func collectAll(client *http.Client, p *progress.Progress, logger LoggerType) ([]*VendorDef, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	registries := Registries()
	defs := make([]*VendorDef, 0)
	errs := make([]error, 0)
	for _, reg := range registries {
		if p != nil {
			p.Advance(uint(88 / len(registries)))
		}
		body, err := fetchCSV(client, reg)
		if err != nil {
			errs = append(errs, err)
			if logger != nil {
				logger.Err(err, "failed to download file '%s'", reg.FileName())
			}
			continue
		}
		results, err := readCSVRows(reg, body, logger)
		body.Close()
		if err != nil {
			return nil, err
		}
		defs = append(defs, results...)
	}
	err := errors.Join(errs...)
	return defs, err
}

func CollectAll(p *progress.Progress, logger LoggerType) ([]*VendorDef, error) {
	return collectAll(nil, p, logger)
}
