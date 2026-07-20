package drive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/proton"
)

// Block-transfer tuning, mirroring the Proton web client's upload worker
// (WebClients packages/drive-store/store/_uploads/constants.ts) so large files
// stream with bounded memory and survive transient failures:
//
//   - blocks are 4 MiB (FILE_CHUNK_SIZE);
//   - only a small window of encrypted blocks is held in memory at once
//     (uploadBufferBlocks + uploadParallelJobs), never the whole file;
//   - upload links are requested in batches as the window drains;
//   - blocks upload in parallel (uploadParallelJobs ~ MAX_UPLOAD_JOBS);
//   - each transfer retries transient failures (blockMaxRetries ~
//     MAX_RETRIES_BEFORE_FAIL), honours 429 Retry-After (bounded by
//     blockMaxRateLimit ~ MAX_TOO_MANY_REQUESTS_WAIT), and refreshes an
//     expired storage token (blockTokenTTL ~ TOKEN_EXPIRATION_TIME).
const (
	driveBlockSize     = 4 * 1024 * 1024
	uploadBufferBlocks = 15
	uploadLinkBatch    = 10
	uploadParallelJobs = 5
	blockMaxRetries    = 3
	blockTransferQuery = 90 * time.Second
	blockTokenTTL      = 3 * time.Hour
	blockMaxRateLimit  = time.Hour
)

// blockRetryBaseDelay is the base backoff between block-transfer retries. It is
// a package var so tests can shrink it; production keeps a real backoff.
var blockRetryBaseDelay = 500 * time.Millisecond

// uploadLink is a block storage upload target plus the time its token was
// issued, so a long-running upload can proactively refresh a stale token.
type uploadLink struct {
	Token   string
	BareURL string
	created time.Time
}

// sleepCtx waits for d, returning early with ctx's error if it is cancelled.
// A non-positive d is a no-op (returns ctx.Err(), nil while the ctx is live).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// backoffDelay is exponential (base, doubling per attempt) capped at 30s.
func backoffDelay(attempt int) time.Duration {
	d := blockRetryBaseDelay << attempt
	if d <= 0 || d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// retryAfter parses a Retry-After header (delta-seconds), bounded to
// blockMaxRateLimit. A missing or malformed header falls back to the backoff
// for attempt.
func retryAfter(header string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
		d := time.Duration(secs) * time.Second
		if d > blockMaxRateLimit {
			d = blockMaxRateLimit
		}
		return d
	}
	return backoffDelay(attempt)
}

// isRetryableAPIErr reports whether a proton-client error is worth retrying: a
// transport failure, or a 5xx / 429 response.
func isRetryableAPIErr(err error) bool {
	var ne *proton.NetworkError
	if errors.As(err, &ne) {
		return true
	}
	var ae *proton.APIError
	if errors.As(err, &ae) {
		return ae.HTTPStatus >= 500 || ae.HTTPStatus == http.StatusTooManyRequests
	}
	return false
}

// tokenRejected reports whether a storage response status means the upload
// token is no longer usable, so a fresh link must be requested (mirrors the web
// client's NOT_FOUND / ALREADY_EXISTS handling).
func tokenRejected(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusConflict, http.StatusGone, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// putBlock uploads one encrypted block's bytes to its storage URL and returns
// the HTTP status, the Retry-After header, and a bounded response body.
func putBlock(ctx context.Context, link uploadLink, data []byte) (status int, retryAfterHdr, body string, err error) {
	const boundary = "proton-cli-boundary"
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"Block\"; filename=\"blob\"\r\n")
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	buf.Write(data)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	req, err := http.NewRequestWithContext(ctx, "POST", link.BareURL, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("pm-storage-token", link.Token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, resp.Header.Get("Retry-After"), string(b), nil
}

// uploadBlock uploads one block, retrying transient failures with backoff,
// honouring 429 Retry-After, and calling refresh for a new link when the
// storage token is rejected or has aged past its TTL. It returns the token that
// the successful upload used, which the revision commit records.
func uploadBlock(ctx context.Context, index int, data []byte, link uploadLink, refresh func(context.Context) (uploadLink, error)) (string, error) {
	for attempt := 0; ; attempt++ {
		if time.Since(link.created) > blockTokenTTL {
			fresh, err := refresh(ctx)
			if err != nil {
				return "", err
			}
			link = fresh
		}

		attemptCtx, cancel := context.WithTimeout(ctx, blockTransferQuery)
		status, ra, body, err := putBlock(attemptCtx, link, data)
		cancel()

		if err == nil && status >= 200 && status < 300 {
			return link.Token, nil
		}
		if attempt >= blockMaxRetries {
			if err != nil {
				return "", fmt.Errorf("upload block %d: %w", index, err)
			}
			return "", fmt.Errorf("upload block %d: HTTP %d: %s", index, status, body)
		}

		switch {
		case err != nil: // network / timeout
			if werr := sleepCtx(ctx, backoffDelay(attempt)); werr != nil {
				return "", werr
			}
		case status == http.StatusTooManyRequests:
			if werr := sleepCtx(ctx, retryAfter(ra, attempt)); werr != nil {
				return "", werr
			}
		case tokenRejected(status): // expired / already-committed token: get a fresh link
			fresh, rerr := refresh(ctx)
			if rerr != nil {
				return "", rerr
			}
			link = fresh
		case status >= 500:
			if werr := sleepCtx(ctx, backoffDelay(attempt)); werr != nil {
				return "", werr
			}
		default: // other 4xx: not recoverable
			return "", fmt.Errorf("upload block %d: HTTP %d: %s", index, status, body)
		}
	}
}

// getBlock downloads one block's bytes, returning the HTTP status and
// Retry-After header on a non-2xx response instead of the body.
func getBlock(ctx context.Context, url, token string) (data []byte, status int, retryAfterHdr string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("pm-storage-token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, resp.StatusCode, resp.Header.Get("Retry-After"), nil
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, "", err
	}
	return b, resp.StatusCode, "", nil
}

// downloadBlock fetches one block's bytes, retrying transient failures with
// backoff and honouring 429 Retry-After.
func downloadBlock(ctx context.Context, url, token string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, blockTransferQuery)
		data, status, ra, err := getBlock(attemptCtx, url, token)
		cancel()

		if err == nil && status >= 200 && status < 300 {
			return data, nil
		}
		if attempt >= blockMaxRetries {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("HTTP %d", status)
		}

		switch {
		case err != nil:
			if werr := sleepCtx(ctx, backoffDelay(attempt)); werr != nil {
				return nil, werr
			}
		case status == http.StatusTooManyRequests:
			if werr := sleepCtx(ctx, retryAfter(ra, attempt)); werr != nil {
				return nil, werr
			}
		case status >= 500:
			if werr := sleepCtx(ctx, backoffDelay(attempt)); werr != nil {
				return nil, werr
			}
		default: // 4xx that isn't rate limiting: not recoverable
			return nil, fmt.Errorf("HTTP %d", status)
		}
	}
}

// requestBlockLinks asks the API for upload links for a batch of blocks. The
// returned links are positional (link i belongs to batch[i]); the call retries
// on transient API failures.
func (s *Service) requestBlockLinks(ctx context.Context, shareID, linkID, revisionID, addrID string, batch []*encBlock) ([]uploadLink, error) {
	blockList := make([]map[string]any, len(batch))
	for i, b := range batch {
		blockList[i] = b.listEntry()
	}
	body := map[string]any{
		"AddressID": addrID, "ShareID": shareID,
		"LinkID": linkID, "RevisionID": revisionID, "BlockList": blockList,
	}
	for attempt := 0; ; attempt++ {
		var res struct {
			UploadLinks []struct{ Token, BareURL string }
		}
		err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/drive/blocks", Body: body}, &res)
		if err == nil {
			if len(res.UploadLinks) != len(batch) {
				return nil, fmt.Errorf("requested %d block links, got %d", len(batch), len(res.UploadLinks))
			}
			now := time.Now()
			links := make([]uploadLink, len(res.UploadLinks))
			for i, l := range res.UploadLinks {
				links[i] = uploadLink{Token: l.Token, BareURL: l.BareURL, created: now}
			}
			return links, nil
		}
		if attempt >= blockMaxRetries || !isRetryableAPIErr(err) {
			return nil, err
		}
		if werr := sleepCtx(ctx, backoffDelay(attempt)); werr != nil {
			return nil, werr
		}
	}
}
