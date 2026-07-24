package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// MediaItem describes one uploaded media file.
type MediaItem struct {
	Name       string `json:"name" dc:"Stored file name"`
	Url        string `json:"url" dc:"Public URL to use in emails"`
	Size       int64  `json:"size" dc:"File size in bytes"`
	UpdateTime int64  `json:"update_time" dc:"Last modified (unix seconds)"`
}

// UploadReq uploads a single image to the shared email-media library.
type UploadReq struct {
	g.Meta        `path:"/email-media/upload" method:"post" tags:"EmailMedia" summary:"Upload an email image" mime:"multipart/form-data" in:"body"`
	Authorization string             `json:"authorization" dc:"Authorization" in:"header"`
	File          *ghttp.UploadFile  `p:"file" type:"file" dc:"Image file (png/jpg/gif/webp/bmp)"`
}

// UploadRes returns the public URL of the uploaded image.
type UploadRes struct {
	api_v1.StandardRes
	Data MediaItem `json:"data"`
}

// ListReq lists the uploaded email images.
type ListReq struct {
	g.Meta        `path:"/email-media/list" method:"get" tags:"EmailMedia" summary:"List email images" in:"query"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

// ListRes returns the media library, newest first.
type ListRes struct {
	api_v1.StandardRes
	Data struct {
		List []MediaItem `json:"list" dc:"Media items"`
	} `json:"data"`
}

// DeleteReq removes one uploaded image from the library.
type DeleteReq struct {
	g.Meta        `path:"/email-media/delete" method:"post" tags:"EmailMedia" summary:"Delete an email image" in:"body"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Name          string `p:"name" v:"required#Please provide the file name" dc:"Stored file name to delete"`
}

// DeleteRes is the response for deleting an image.
type DeleteRes struct {
	api_v1.StandardRes
}
