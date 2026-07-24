package email_media

import (
	v1 "billionmail-core/api/email_media/v1"
	"billionmail-core/internal/service/public"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// mediaDirName is the sub-directory (under the core data dir) where uploaded
// email images are stored. The core-data volume is persistent, so images
// survive redeploys. Files here are served publicly at /media/<name> (see the
// AddStaticPath registration in cmd.go).
const mediaDirName = "data/email-media"

// maxUploadBytes caps a single upload at 5 MB.
const maxUploadBytes int64 = 5 * 1024 * 1024

// allowedExt is the set of accepted image extensions. SVG is deliberately
// excluded: it can embed <script>, and /media is served publicly on the
// panel's own origin (stored-XSS risk); mail clients strip SVG anyway.
var allowedExt = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

func mediaDir() string {
	return public.AbsPath(mediaDirName)
}

// publicURL builds an absolute, publicly-reachable URL for a stored file so it
// renders in a recipient's inbox. Honours a reverse-proxy's X-Forwarded-Proto.
func publicURL(ctx context.Context, name string) string {
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return "/media/" + name
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host + "/media/" + name
}

// Upload stores an uploaded image and returns its public URL.
func (c *ControllerV1) Upload(ctx context.Context, req *v1.UploadReq) (res *v1.UploadRes, err error) {
	res = &v1.UploadRes{}

	if req.File == nil {
		res.SetError(gerror.New("No file was uploaded"))
		return res, nil
	}
	if req.File.Size > maxUploadBytes {
		res.SetError(gerror.New("Image is too large (max 5 MB)"))
		return res, nil
	}

	ext := strings.ToLower(filepath.Ext(req.File.Filename))
	if !allowedExt[ext] {
		res.SetError(gerror.New("Unsupported image type; allowed: png, jpg, gif, webp, bmp"))
		return res, nil
	}

	dir := mediaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.SetError(gerror.New("Failed to prepare media directory"))
		return res, nil
	}

	// Save with a random name to avoid collisions and hide the original name.
	filename, err := req.File.Save(dir, true)
	if err != nil {
		res.SetError(gerror.New("Failed to save the image"))
		return res, nil
	}

	res.Data = v1.MediaItem{
		Name: filename,
		Url:  publicURL(ctx, filename),
		Size: req.File.Size,
	}
	res.SetSuccess("Image uploaded successfully")
	return res, nil
}

// List returns every image in the library, newest first.
func (c *ControllerV1) List(ctx context.Context, req *v1.ListReq) (res *v1.ListRes, err error) {
	res = &v1.ListRes{}

	dir := mediaDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// No directory yet just means an empty library.
		res.Data.List = []v1.MediaItem{}
		res.SetSuccess("Success")
		return res, nil
	}

	items := make([]v1.MediaItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !allowedExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		items = append(items, v1.MediaItem{
			Name:       e.Name(),
			Url:        publicURL(ctx, e.Name()),
			Size:       info.Size(),
			UpdateTime: info.ModTime().Unix(),
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].UpdateTime > items[j].UpdateTime })

	res.Data.List = items
	res.SetSuccess("Success")
	return res, nil
}

// Delete removes one image from the library. The name is reduced to its base to
// prevent path traversal.
func (c *ControllerV1) Delete(ctx context.Context, req *v1.DeleteReq) (res *v1.DeleteRes, err error) {
	res = &v1.DeleteRes{}

	name := filepath.Base(strings.TrimSpace(req.Name))
	if name == "" || name == "." || name == ".." {
		res.SetError(gerror.New("Invalid file name"))
		return res, nil
	}

	target := filepath.Join(mediaDir(), name)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		res.SetError(gerror.New("Failed to delete the image"))
		return res, nil
	}

	res.SetSuccess("Image deleted successfully")
	return res, nil
}
