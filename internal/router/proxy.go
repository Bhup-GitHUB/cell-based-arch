package router

import (
	"bytes"
	"io"
	"net/http"
)

func forward(w http.ResponseWriter, r *http.Request, targetURL string, body []byte) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error":"failed to build upstream request"}`, http.StatusInternalServerError)
		return
	}

	for key, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}
	req.Header.Set("Content-Length", r.Header.Get("Content-Length"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
