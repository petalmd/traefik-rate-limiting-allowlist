package ratelimiterallowlist

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
	"github.com/traefik/traefik/v2/pkg/testhelpers"
	"github.com/vulcand/oxy/utils"
)

func TestNewRateLimiterAllowList(t *testing.T) {
	testCases := []struct {
		desc             string
		config           *Config
		expectedMaxDelay time.Duration
		expectedSourceIP string
		requestHeader    string
		expectedError    string
	}{
		{
			desc: "maxDelay computation",
			config: &Config{
				Average: 200,
				Burst:   10,
			},
			expectedMaxDelay: 2500 * time.Microsecond,
		},
		{
			desc: "maxDelay computation, low rate regime",
			config: &Config{
				Average: 2,
				Period:  ptypes.Duration(10 * time.Second),
				Burst:   10,
			},
			expectedMaxDelay: 500 * time.Millisecond,
		},
		{
			desc: "default SourceMatcher is remote address ip strategy",
			config: &Config{
				Average: 200,
				Burst:   10,
			},
			expectedSourceIP: "127.0.0.1",
		},
		{
			desc: "SourceCriterion in config is respected",
			config: &Config{
				Average: 200,
				Burst:   10,
				SourceCriterion: &dynamic.SourceCriterion{
					RequestHeaderName: "Foo",
				},
			},
			requestHeader: "bar",
		},
		{
			desc: "SourceCriteria are mutually exclusive",
			config: &Config{
				Average: 200,
				Burst:   10,
				SourceCriterion: &dynamic.SourceCriterion{
					IPStrategy:        &dynamic.IPStrategy{},
					RequestHeaderName: "Foo",
				},
			},
			expectedError: "iPStrategy and RequestHeaderName are mutually exclusive",
		},
		{
			desc: "Exclusion SourceRange is an invalid CIDR",
			config: &Config{
				Average: 200,
				Burst:   10,
				SourceCriterion: &dynamic.SourceCriterion{
					IPStrategy:        &dynamic.IPStrategy{},
					RequestHeaderName: "Foo",
				},
				Exclusion: &Exclusion{
					SourceRange: []string{"foo"},
				},
			},
			expectedError: "cannot parse CIDR [foo]: parsing CIDR trusted IPs <nil>: invalid CIDR address: foo",
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

			h, err := New(context.Background(), next, test.config, "rate-limiter")
			if test.expectedError != "" {
				assert.EqualError(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}

			rtl, _ := h.(*rateLimiterAllowList)
			if test.expectedMaxDelay != 0 {
				assert.Equal(t, test.expectedMaxDelay, rtl.maxDelay)
			}

			if test.expectedSourceIP != "" {
				extractor, ok := rtl.sourceMatcher.(utils.ExtractorFunc)
				require.True(t, ok, "Not an ExtractorFunc")

				req := http.Request{
					RemoteAddr: fmt.Sprintf("%s:1234", test.expectedSourceIP),
				}

				ip, _, err := extractor(&req)
				assert.NoError(t, err)
				assert.Equal(t, test.expectedSourceIP, ip)
			}
			if test.requestHeader != "" {
				extractor, ok := rtl.sourceMatcher.(utils.ExtractorFunc)
				require.True(t, ok, "Not an ExtractorFunc")

				req := http.Request{
					Header: map[string][]string{
						test.config.SourceCriterion.RequestHeaderName: {test.requestHeader},
					},
				}
				hd, _, err := extractor(&req)
				assert.NoError(t, err)
				assert.Equal(t, test.requestHeader, hd)
			}
		})
	}
}

func TestRateLimitAllowlist(t *testing.T) {
	testCases := []struct {
		desc         string
		config       *Config
		loadDuration time.Duration
		incomingLoad int // in reqs/s
		burst        int
		overCharge   bool
	}{
		{
			desc: "Average is respected",
			config: &Config{
				Average: 100,
				Burst:   1,
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 400,
			overCharge:   false,
		},
		{
			desc: "burst allowed, no bursty traffic",
			config: &Config{
				Average: 100,
				Burst:   100,
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			overCharge:   false,
		},
		{
			desc: "burst allowed, initial burst, under capacity",
			config: &Config{
				Average: 100,
				Burst:   100,
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			burst:        50,
			overCharge:   false,
		},
		{
			desc: "burst allowed, initial burst, over capacity",
			config: &Config{
				Average: 100,
				Burst:   100,
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			burst:        150,
			overCharge:   false,
		},
		{
			desc: "burst over average, initial burst, over capacity",
			config: &Config{
				Average: 100,
				Burst:   200,
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			burst:        300,
			overCharge:   false,
		},
		{
			desc: "lower than 1/s",
			config: &Config{
				Average: 5,
				Period:  ptypes.Duration(10 * time.Second),
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 100,
			burst:        0,
			overCharge:   false,
		},
		{
			desc: "lower than 1/s, longer",
			config: &Config{
				Average: 5,
				Period:  ptypes.Duration(10 * time.Second),
			},
			loadDuration: time.Minute,
			incomingLoad: 100,
			burst:        0,
			overCharge:   false,
		},
		{
			desc: "lower than 1/s, longer, harsher",
			config: &Config{
				Average: 1,
				Period:  ptypes.Duration(time.Minute),
			},
			loadDuration: time.Minute,
			incomingLoad: 100,
			burst:        0,
			overCharge:   false,
		},
		{
			desc: "period below 1 second",
			config: &Config{
				Average: 50,
				Period:  ptypes.Duration(500 * time.Millisecond),
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 300,
			burst:        0,
			overCharge:   false,
		},
		{
			desc: "over capacity, but not with the exclusion SourceRange",
			config: &Config{
				Average: 100,
				Burst:   0,
				Exclusion: &Exclusion{
					SourceRange: []string{"127.0.0.1"},
				},
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			burst:        50,
			overCharge:   false,
		},
		{
			desc: "exclusion SourceRange not matching, over charge",
			config: &Config{
				Average: 100,
				Burst:   0,
				Exclusion: &Exclusion{
					SourceRange: []string{"192.168.1.1"},
				},
			},
			loadDuration: 2 * time.Second,
			incomingLoad: 200,
			burst:        0,
			overCharge:   true,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			if test.loadDuration >= time.Minute && testing.Short() {
				t.Skip("skipping test in short mode.")
			}
			t.Parallel()

			reqCount := 0
			dropped := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqCount++
			})
			h, err := New(context.Background(), next, test.config, "rate-limiter")
			require.NoError(t, err)

			loadPeriod := time.Duration(1e9 / test.incomingLoad)
			start := time.Now()
			end := start.Add(test.loadDuration)
			ticker := time.NewTicker(loadPeriod)
			defer ticker.Stop()
			for {
				if time.Now().After(end) {
					break
				}

				req := testhelpers.MustNewRequest(http.MethodGet, "http://localhost", nil)
				req.RemoteAddr = "127.0.0.1:1234"
				w := httptest.NewRecorder()

				h.ServeHTTP(w, req)
				if w.Result().StatusCode != http.StatusOK {
					dropped++
				}
				if test.burst > 0 && reqCount < test.burst {
					// if a burst is defined we first hammer the server with test.burst requests as fast as possible
					continue
				}
				<-ticker.C
			}
			stop := time.Now()
			elapsed := stop.Sub(start)

			burst := test.config.Burst
			if burst < 1 {
				// actual default value
				burst = 1
			}
			period := time.Duration(test.config.Period)
			if period == 0 {
				period = time.Second
			}
			if test.config.Average == 0 {
				if reqCount < 75*test.incomingLoad/100 {
					t.Fatalf("we (arbitrarily) expect at least 75%% of the requests to go through with no rate limiting, and yet only %d/%d went through", reqCount, test.incomingLoad)
				}
				if dropped != 0 {
					t.Fatalf("no request should have been dropped if rate limiting is disabled, and yet %d were", dropped)
				}
				return
			}

			// Note that even when there is no bursty traffic,
			// we take into account the configured burst,
			// because it also helps absorbing non-bursty traffic.
			rate := float64(test.config.Average) / float64(period)
			wantCount := int(int64(rate*float64(test.loadDuration)) + burst)
			// Allow for a 2% leeway
			maxCount := wantCount * 102 / 100
			// With very high CPU loads,
			// we can expect some extra delay in addition to the rate limiting we already do,
			// so we allow for some extra leeway there.
			// Feel free to adjust wrt to the load on e.g. the CI.
			minCount := wantCount * 95 / 100
			if reqCount < minCount {
				t.Fatalf("rate was slower than expected: %d requests (wanted > %d) in %v", reqCount, minCount, elapsed)
			}
			if reqCount > maxCount {
				if test.overCharge != true {
					t.Fatalf("rate was faster than expected: %d requests (wanted < %d) in %v", reqCount, maxCount, elapsed)
				}
			} else if test.overCharge == true {
				t.Fatalf("dropped call expected: %d requests (wanted < %d) in %v", reqCount, maxCount, elapsed)
			}
		})
	}
}
