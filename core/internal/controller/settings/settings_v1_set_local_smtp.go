package settings

import (
	"billionmail-core/internal/service/public"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"

	"billionmail-core/api/settings/v1"
)

func (c *ControllerV1) SetLocalSMTP(ctx context.Context, req *v1.SetLocalSMTPReq) (res *v1.SetLocalSMTPRes, err error) {
	res = &v1.SetLocalSMTPRes{}

	err = public.OptionsMgrInstance.SetOption(ctx, "local_smtp_enabled", req.Enabled)
	if err != nil {
		res.SetError(gerror.Newf(public.LangCtx(ctx, "Failed to set local SMTP switch: %v", err)))
		return res, nil
	}

	res.SetSuccess(public.LangCtx(ctx, "Local SMTP switch updated successfully"))
	return res, nil
}
