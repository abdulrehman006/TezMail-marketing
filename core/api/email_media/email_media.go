// Package email_media provides a shared media library for email images:
// upload once, get a public /media/<file> URL, list and delete.
package email_media

import (
	"context"

	v1 "billionmail-core/api/email_media/v1"
)

type IEmailMediaV1 interface {
	Upload(ctx context.Context, req *v1.UploadReq) (res *v1.UploadRes, err error)
	List(ctx context.Context, req *v1.ListReq) (res *v1.ListRes, err error)
	Delete(ctx context.Context, req *v1.DeleteReq) (res *v1.DeleteRes, err error)
}
