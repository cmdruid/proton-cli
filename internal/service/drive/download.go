package drive

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/progress"
	"github.com/roman-16/proton-cli/internal/proton"
)

type DownloadOptions struct {
	// Label names the transfer for the progress report.
	Label string
	// Progress receives byte counts; nil discards them.
	Progress progress.Sink
}

func (s *Service) Download(ctx context.Context, dc *Context, path string, w io.Writer, opts DownloadOptions) error {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return err
	}
	if res.IsFolder {
		return fmt.Errorf("%s is a folder, not a file", path)
	}
	link, err := s.getLink(ctx, res.ShareID, res.LinkID)
	if err != nil {
		return err
	}
	return s.downloadFile(ctx, res.ShareID, link, res.NodeKR, w, opts)
}

// downloadFile streams and decrypts the active revision of a file link whose
// node key ring (nodeKR) has already been unwrapped.
func (s *Service) downloadFile(ctx context.Context, shareID string, link *Link, nodeKR *pgp.KeyRing, w io.Writer, opts DownloadOptions) error {
	if link.FileProperties == nil {
		return fmt.Errorf("%s: no file properties", link.LinkID)
	}
	kp, err := base64.StdEncoding.DecodeString(link.FileProperties.ContentKeyPacket)
	if err != nil {
		return err
	}
	sk, err := nodeKR.DecryptSessionKey(kp)
	if err != nil {
		return fmt.Errorf("get file session key: %w", err)
	}

	// Revision blocks are paginated; page through them all so files larger than
	// one page (PageSize blocks) download in full rather than truncating.
	type revBlock struct {
		Index        int
		BareURL      string
		Token        string
		EncSignature string
	}
	const pageSize = 50
	revID := link.FileProperties.ActiveRevision.ID
	var blocks []revBlock
	for from := 1; ; from += pageSize {
		var rev struct {
			Revision struct{ Blocks []revBlock }
		}
		q := proton.Request{
			Method: "GET",
			Path:   fmt.Sprintf("/drive/shares/%s/files/%s/revisions/%s", shareID, link.LinkID, revID),
		}
		q.Query = make(map[string][]string)
		q.Query.Set("FromBlockIndex", strconv.Itoa(from))
		q.Query.Set("PageSize", strconv.Itoa(pageSize))
		if err := s.C.Decode(ctx, q, &rev); err != nil {
			return err
		}
		blocks = append(blocks, rev.Revision.Blocks...)
		if len(rev.Revision.Blocks) < pageSize {
			break
		}
	}

	prog := progress.Of(opts.Progress)
	prog.Start(link.Size, opts.Label)
	defer prog.Done()

	for i, b := range blocks {
		encData, err := downloadBlock(ctx, b.BareURL, b.Token)
		if err != nil {
			return fmt.Errorf("download block %d: %w", i+1, err)
		}
		dec, err := sk.Decrypt(encData)
		if err != nil {
			return fmt.Errorf("decrypt block %d: %w", i+1, err)
		}
		bin := dec.GetBinary()
		if _, err := w.Write(bin); err != nil {
			return err
		}
		prog.Add(int64(len(bin)))
	}
	return nil
}
