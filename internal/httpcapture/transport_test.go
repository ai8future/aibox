package httpcapture

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

type mockTransport struct {
	roundTripFunc func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestTransport_RoundTrip(t *testing.T) {
	reqBody := []byte("request payload")
	respBody := []byte("response payload")

	mock := &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Verify request body is readable
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(body, reqBody) {
				t.Errorf("expected request body %q, got %q", reqBody, body)
			}
			req.Body.Close()

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}, nil
		},
	}

	tr := New()
	tr.Base = mock

	req, err := http.NewRequest("POST", "http://example.com", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify request captured
	if !bytes.Equal(tr.RequestBody, reqBody) {
		t.Errorf("expected captured request %q, got %q", reqBody, tr.RequestBody)
	}

	// Verify response captured
	if !bytes.Equal(tr.ResponseBody, respBody) {
		t.Errorf("expected captured response %q, got %q", respBody, tr.ResponseBody)
	}

	// Verify response body is still readable
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if !bytes.Equal(body, respBody) {
		t.Errorf("expected read response %q, got %q", respBody, body)
	}
}

func TestTransport_Client(t *testing.T) {
	tr := New()
	client := tr.Client()
	if client.Transport != tr {
		t.Error("client transport mismatch")
	}
}
