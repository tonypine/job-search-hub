// Package exchangerates reads the day's ECB reference rates from Frankfurter,
// a free keyless API, and keeps them for a day per base currency.
package exchangerates

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// DefaultAPIBase is Frankfurter's API root.
	DefaultAPIBase = "https://api.frankfurter.dev/v1"
	// freshness is how long fetched rates are used; ECB publishes once a day.
	freshness      = 12 * time.Hour
	requestTimeout = 10 * time.Second
)

type Cache struct {
	apiBase string
	client  *http.Client
	now     func() time.Time

	mutex   sync.Mutex
	fetched map[string]fetchedRates
}

type fetchedRates struct {
	perBase map[string]float64
	at      time.Time
}

func NewCache(apiBase string) *Cache {
	return &Cache{apiBase: apiBase, client: &http.Client{Timeout: requestTimeout}, now: time.Now, fetched: map[string]fetchedRates{}}
}

// GetRates returns how much of each currency one unit of base buys, fetching
// them when the cached ones are older than half a day.
func (cache *Cache) GetRates(ctx context.Context, base string) (map[string]float64, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cached, found := cache.fetched[base]; found && cache.now().Sub(cached.at) < freshness {
		return cached.perBase, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cache.apiBase+"/latest?"+url.Values{"base": {base}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := cache.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach the exchange-rate API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the exchange-rate API answered %d for base %s", response.StatusCode, base)
	}
	var answer struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		return nil, fmt.Errorf("read the exchange rates: %w", err)
	}
	cache.fetched[base] = fetchedRates{perBase: answer.Rates, at: cache.now()}
	return answer.Rates, nil
}
