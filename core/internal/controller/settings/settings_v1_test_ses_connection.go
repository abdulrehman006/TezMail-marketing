package settings

import (
	"billionmail-core/api/settings/v1"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/ses_api"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) TestSESConnection(ctx context.Context, req *v1.TestSESConnectionReq) (res *v1.TestSESConnectionRes, err error) {
	res = &v1.TestSESConnectionRes{}

	// Validate required fields
	if req.Region == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS region is required")))
		return res, nil
	}
	if req.AccessKey == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS access key is required")))
		return res, nil
	}
	if req.SecretKey == "" {
		res.SetError(gerror.New(public.LangCtx(ctx, "AWS secret key is required")))
		return res, nil
	}

	// Test the connection
	status, verifiedDomains, quota, err := ses_api.TestSESCredentials(ctx, req.Region, req.AccessKey, req.SecretKey)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Connection test failed: {}", err)))
		return res, nil
	}

	res.Data.Status = status
	res.Data.VerifiedDomains = verifiedDomains
	if quota != nil {
		res.Data.SendQuota = &v1.SESQuota{
			Max24HourSend:   quota.Max24HourSend,
			MaxSendRate:     quota.MaxSendRate,
			SentLast24Hours: quota.SentLast24Hours,
		}
	}

	res.SetSuccess(public.LangCtx(ctx, "Connection test successful"))
	return res, nil
}
