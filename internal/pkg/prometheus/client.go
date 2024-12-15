/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package prometheus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"time"
)

type PrometheusClient struct {
	timeout  time.Duration
	url      url.URL
	username string
	password string
	token    string
	client   *http.Client
}

func NewPrometheusClient(endpoint string, insecureSkipVerify bool, username, password, token string) (*PrometheusClient, error) {
	promURL, err := url.Parse(endpoint)
	if endpoint == "" || err != nil {
		return nil, fmt.Errorf("address %s is not a valid URL", endpoint)
	}

	prom := PrometheusClient{
		timeout: 5 * time.Second,
		url:     *promURL,
		client:  http.DefaultClient,
	}

	if insecureSkipVerify {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		prom.client = &http.Client{Transport: t}
	}

	if (username != "" && password == "") || (username == "" && password != "") {
		return nil, fmt.Errorf("if password or username are used for basic authentication, both should be a non-empty string")
	}
	if username != "" && token != "" {
		return nil, fmt.Errorf("you can either configure the token for bearer authentication or the username and password for basic authentication, not both")
	}

	prom.username = username
	prom.password = password
	prom.token = token

	return &prom, nil
}

type prometheusResponse struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name string `json:"name"`
			}
			Value []interface{} `json:"value"`
		}
	}
}

// RunQuery executes the promQL query and returns true if the query returned data.
func (p *PrometheusClient) RunQuery(query string) (bool, error) {
	query = url.QueryEscape(trimQuery(query))
	u, err := url.Parse(fmt.Sprintf("./api/v1/query?query=%s", query))
	if err != nil {
		return false, fmt.Errorf("url.Parse failed: %w", err)
	}
	u.Path = path.Join(p.url.Path, u.Path)

	u = p.url.ResolveReference(u)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("http.NewRequest failed: %w", err)
	}

	if p.username != "" && p.password != "" {
		req.SetBasicAuth(p.username, p.password)
	}

	if p.token != "" {
		req.Header.Add("Authorization", "Bearer "+p.token)
	} else if p.username != "" && p.password != "" {
		req.SetBasicAuth(p.username, p.password)
	}

	ctx, cancel := context.WithTimeout(req.Context(), p.timeout)
	defer cancel()

	r, err := p.client.Do(req.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = r.Body.Close()
	}()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return false, fmt.Errorf("error reading body: %w", err)
	}

	if 400 <= r.StatusCode {
		return false, fmt.Errorf("error response: %s", string(b))
	}

	var result prometheusResponse
	err = json.Unmarshal(b, &result)
	if err != nil {
		return false, fmt.Errorf("error unmarshaling result: %w, '%s'", err, string(b))
	}

	if len(result.Data.Result) > 0 {
		return true, nil
	}
	return false, nil
}

// trimQuery takes a promql query and removes whitespace.
func trimQuery(query string) string {
	space := regexp.MustCompile(`\s+`)
	return space.ReplaceAllString(query, " ")
}
