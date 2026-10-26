package indexnow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// requestTimeout bounds one POST, connection to last byte.
	requestTimeout = 30 * time.Second

	// defaultRetryBackoff is the wait before the one retry a 5xx or a network
	// failure gets.
	defaultRetryBackoff = 5 * time.Second

	// maxLoggedBodyBytes bounds how much of a rejection's body is logged.
	maxLoggedBodyBytes = 1024
)

// submission is the protocol's JSON body for one batch.
type submission struct {
	Host        string   `json:"host"`
	Key         string   `json:"key"`
	KeyLocation string   `json:"keyLocation"`
	URLList     []string `json:"urlList"`
}

// submitter posts batches to one endpoint.
type submitter struct {
	client       *http.Client
	endpoint     string
	retryBackoff time.Duration
	logger       *slog.Logger
}

// newSubmitter returns a submitter whose client follows no redirect: a 3xx is
// answered as the response it is, so a request never leaves the endpoint's
// host.
func newSubmitter(endpoint string, logger *slog.Logger) *submitter {
	return &submitter{
		client: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		endpoint:     endpoint,
		retryBackoff: defaultRetryBackoff,
		logger:       logger,
	}
}

// submit posts one batch and returns the last HTTP status it saw, 0 when no
// response arrived. 200 and 202 succeed. Any other status below 500 fails at
// once and its body is logged. A 5xx or a network failure is retried once,
// after retryBackoff, and fails if the retry does not succeed.
func (s *submitter) submit(ctx context.Context, body submission) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("encode indexnow submission: %w", err)
	}
	var status int
	for attempt := 1; ; attempt++ {
		var respBody []byte
		status, respBody, err = s.post(ctx, payload)
		switch {
		case err == nil && (status == http.StatusOK || status == http.StatusAccepted):
			return status, nil
		case err == nil && status < http.StatusInternalServerError:
			s.logger.Warn("indexnow: submission rejected",
				"status", status,
				"urls", len(body.URLList),
				"body", string(respBody),
			)
			return status, fmt.Errorf("indexnow rejected the submission: HTTP %d", status)
		}
		if err == nil {
			err = fmt.Errorf("indexnow answered HTTP %d", status)
		}
		if attempt == 2 || ctx.Err() != nil {
			return status, err
		}
		s.logger.Warn("indexnow: submission failed, retrying once",
			"status", status,
			"error", err,
			"backoff", s.retryBackoff,
		)
		timer := time.NewTimer(s.retryBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return status, ctx.Err()
		case <-timer.C:
		}
	}
}

// post sends one request and returns its status and the first
// maxLoggedBodyBytes of its body.
func (s *submitter) post(ctx context.Context, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("build indexnow request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("post indexnow submission: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxLoggedBodyBytes))
	return resp.StatusCode, body, nil
}
